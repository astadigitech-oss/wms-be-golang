package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"liquid8/wms/config"
	"liquid8/wms/helpers"
	"liquid8/wms/models"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

type OlseraService struct {
	base_url	string
	client      *http.Client
	destination *models.MigrateColorDestination
	logger      *logrus.Logger
}

func NewOlseraService(dest *models.MigrateColorDestination, logger *logrus.Logger) *OlseraService {
	return &OlseraService{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		destination: dest,
		logger:      logger,
		base_url: os.Getenv("OLSERA_BASE_URL"),
	}
}

func (s *OlseraService) getToken(ctx context.Context) (string, error) {

	if s.destination.OlseraAccessToken != nil && !s.destination.IsTokenExpired() {
		return *s.destination.OlseraAccessToken, nil
	}

	if s.destination.OlseraRefreshToken != nil {
		token, err := s.refreshToken(ctx)
		if err == nil {
			return token, nil
		}
	}

	// return "", fmt.Errorf("Mencoba request token baru")
	return s.requestNewToken(ctx)
}

func (s *OlseraService) saveToken() error {

	if s.destination.ID == 0 {
		return errors.New("destination ID is required")
	}

	updates := map[string]interface{}{
		"olsera_access_token":      s.destination.OlseraAccessToken,
		"olsera_refresh_token":     s.destination.OlseraRefreshToken,
		"olsera_token_expires_at":  s.destination.OlseraTokenExpiresAt,
	}

	if err := config.DB.Model(&s.destination).
		Updates(updates).Error; err != nil {

		s.logger.WithError(err).Error("Failed to save Olsera token")
		return err
	}

	return nil
}

func (s *OlseraService) requestNewToken(ctx context.Context) (string, error) {
	s.logger.Info("Request New Token")
	params := url.Values{}
	params.Set("grant_type", "secret_key")
	params.Set("app_id", s.destination.OlseraAppID)
	secret_key, err := helpers.Decrypt(s.destination.OlseraSecretKey)
	if err != nil {
		return "", fmt.Errorf("Gagal decrypt secret key: %s", err.Error())
	}
	params.Set("secret_key", secret_key)

	return s.performAuthRequest(ctx, params, "INITIAL_AUTH")
}

func (s *OlseraService) refreshToken(ctx context.Context) (string, error) {
	s.logger.Info("Request Refresh Token")
	params := url.Values{}
	params.Set("grant_type", "refresh_token")
	params.Set("refresh_token", *s.destination.OlseraRefreshToken)

	return s.performAuthRequest(ctx, params, "REFRESH_AUTH")
}

type apiResponse struct {
	Success    bool        `json:"success"`
	Data       interface{} `json:"data"`
	Message		string		`json:"message"`
	StatusCode int         `json:"status_code"`
}

func (s *OlseraService) performAuthRequest(ctx context.Context, params url.Values, tag string) (string, error) {

	authURL := strings.TrimRight(s.base_url, "/") + "/id/token"

	req, err := http.NewRequestWithContext(ctx, "POST", authURL, strings.NewReader(params.Encode()))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		s.logger.WithError(err).Errorf("[OLSERA-%s] HTTP Error", tag)
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result map[string]interface{}
	_ = json.Unmarshal(body, &result)

	if accessToken, ok := result["access_token"].(string); ok {

		expiresIn := int64(86000)
		if val, ok := result["expires_in"].(float64); ok {
			expiresIn = int64(val/2) - 60
		}	

		s.destination.OlseraAccessToken = &accessToken
		if refresh, ok := result["refresh_token"].(string); ok {
			s.destination.OlseraRefreshToken = &refresh
		}

		location,_ := time.LoadLocation("Asia/Jakarta")
		expires_at := time.Now().In(location).Add(time.Duration(expiresIn) * time.Second)
		s.destination.OlseraTokenExpiresAt = &expires_at

		if err := s.saveToken(); err != nil {
			return "", err
		}

		s.logger.WithFields(logrus.Fields{
			"shop": s.destination.ShopName,
			"expires_in": expiresIn,
		}).Info("Request new token successfully")
		return accessToken, nil
	}

	s.logger.WithFields(logrus.Fields{
		"shop": s.destination.ShopName,
		"resp": string(body),
	}).Errorf("[OLSERA-%s] Auth Failed", tag)

	return "", errors.New("failed authenticate to Olsera")
}

func (s *OlseraService) sendRequest(ctx context.Context, method, endpoint string, payload interface{}) (apiResponse, error) {

	token, err := s.getToken(ctx)
	if err != nil {
		return apiResponse{}, err
	}

	fullURL := strings.TrimRight(s.base_url, "/") + "/en/" + endpoint

	var reqBody io.Reader

	if method == http.MethodGet && payload != nil {
		query := url.Values{}
		for k, v := range payload.(map[string]string) {
			query.Set(k, v)
		}
		fullURL += "?" + query.Encode()
	} else if payload != nil {
		jsonBytes, _ := json.Marshal(payload)
		reqBody = bytes.NewBuffer(jsonBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, fullURL, reqBody)
	if err != nil {
		return apiResponse{}, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		s.logger.WithError(err).Error("[OLSERA] Request Failed")
		return apiResponse{}, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return apiResponse{}, fmt.Errorf("invalid JSON response: %s", string(body))
	}

	s.logger.WithFields(logrus.Fields{
		"store":  s.destination.ShopName,
		"method": method,
		"url":    endpoint,
		"status": resp.StatusCode,
	}).Info("[OLSERA] API Call")

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return apiResponse{
			Success: true,
			Data: result,
			StatusCode: resp.StatusCode,
		}, nil
		// return map[string]interface{}{
		// 	"success":     true,
		// 	"data":        result,
		// 	"status_code": resp.StatusCode,
		// }, nil
	}

	return apiResponse{Success: false, StatusCode: resp.StatusCode}, fmt.Errorf("%s", string(body))
}

func (s *OlseraService) CreateStockInOut(ctx context.Context, data interface{}) (apiResponse, error) {
	return s.sendRequest(ctx, http.MethodPost, "inventory/stockinout", data)
}

func (s *OlseraService) AddItemStockInOut(ctx context.Context, data interface{}) (apiResponse, error) {
	return s.sendRequest(ctx, http.MethodPost, "inventory/stockinout/additem", data)
}

func (s *OlseraService) UpdateStatusStockInOut(ctx context.Context, data interface{}) (apiResponse, error) {
	return s.sendRequest(ctx, http.MethodPost, "inventory/stockinout/updatestatus", data)
}

func (s *OlseraService) GetProductList(ctx context.Context, params map[string]string) (apiResponse, error) {
	return s.sendRequest(ctx, http.MethodGet, "product", params)
}

func (s *OlseraService) GetOutgoingStockList(ctx context.Context, params map[string]string) (apiResponse, error) {
	return s.sendRequest(ctx, http.MethodGet, "inventory/stockoutgoing", params)
}
func (s *OlseraService) GetDetailOutgoingStock(ctx context.Context, params map[string]string) (apiResponse, error) {
	return s.sendRequest(ctx, http.MethodGet, "inventory/stockinout/detail", params)
}

func (s *OlseraService) SyncOlseraToken(ctx context.Context) error {
	_, err := s.requestNewToken(ctx)
	return err
}

//============= cara pakai

// dest := &olsera.Destination{
// 	ShopName:       "Toko A",
// 	BaseURL:        "https://api-open.olsera.co.id/api/open-api/v1",
// 	AppID:          "APP_ID_KAMU",
// 	SecretKey:      "SECRET_KEY_KAMU",
// 	AccessToken:    "",
// 	RefreshToken:   "",
// 	TokenExpiresAt: time.Time{},
// }

// olseraService := olsera.NewOlseraService(dest, logger)

// func GetProductsHandler(c *gin.Context) {

// 	ctx := c.Request.Context()

// 	result, err := olseraService.GetProductList(ctx, map[string]string{
// 		"page":  "1",
// 		"limit": "10",
// 	})

// 	if err != nil {
// 		c.JSON(500, gin.H{
// 			"success": false,
// 			"message": err.Error(),
// 		})
// 		return
// 	}

// 	c.JSON(200, gin.H{
// 		"success": true,
// 		"data":    result,
// 	})
// }