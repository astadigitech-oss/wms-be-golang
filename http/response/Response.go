package response

type ResponseResource struct {
	Status  bool        `json:"status"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func Success(message string, data interface{}) ResponseResource {
	return ResponseResource{
		Status:  true,
		Message: message,
		Data:    data,
	}
}

func Error(message string) ResponseResource {
	return ResponseResource{
		Status:  false,
		Message: message,
	}
}
