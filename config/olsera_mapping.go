package config

import "fmt"

type OlseraTagItem struct {
	Tag      string `json:"tag"`
	OlseraID string `json:"olsera_id"`
	Type     string `json:"type"`
}

var DiskonterData = map[string][]OlseraTagItem{
	"Diskonter Proklamasi": {
		{Tag: "kuning", OlseraID: "111150295", Type: "color_tag"},
		{Tag: "hijau", OlseraID: "111150295", Type: "color_tag"},
		{Tag: "small", OlseraID: "111150295", Type: "sku_product"},
		{Tag: "merah", OlseraID: "111150294", Type: "color_tag"},
		{Tag: "biru", OlseraID: "111150294", Type: "color_tag"},
		{Tag: "big", OlseraID: "111150294", Type: "sku_product"},
	},
	// "Diskonter Pinang": {
	// 	{Tag: "kuning", OlseraID: "85925026", Type: "color_tag"},
	// 	{Tag: "hijau", OlseraID: "85925026", Type: "color_tag"},
	// 	{Tag: "small", OlseraID: "85925026", Type: "sku_product"},
	// 	{Tag: "merah", OlseraID: "85925027", Type: "color_tag"},
	// 	{Tag: "biru", OlseraID: "85925027", Type: "color_tag"},
	// 	{Tag: "big", OlseraID: "85925027", Type: "sku_product"},
	// },
	// "Diskonter Cinere": {
	// 	{Tag: "kuning", OlseraID: "83372506", Type: "color_tag"},
	// 	{Tag: "hijau", OlseraID: "83372506", Type: "color_tag"},
	// 	{Tag: "small", OlseraID: "83372506", Type: "sku_product"},
	// 	{Tag: "merah", OlseraID: "83372507", Type: "color_tag"},
	// 	{Tag: "biru", OlseraID: "83372507", Type: "color_tag"},
	// 	{Tag: "big", OlseraID: "83372507", Type: "sku_product"},
	// },
	// "Diskonter Kayu Manis": {
	// 	{Tag: "kuning", OlseraID: "82547328", Type: "color_tag"},
	// 	{Tag: "hijau", OlseraID: "82547328", Type: "color_tag"},
	// 	{Tag: "small", OlseraID: "82547328", Type: "sku_product"},
	// 	{Tag: "merah", OlseraID: "82547348", Type: "color_tag"},
	// 	{Tag: "biru", OlseraID: "82547348", Type: "color_tag"},
	// 	{Tag: "big", OlseraID: "82547348", Type: "sku_product"},
	// },
	// "Diskonter Zambrud": {
	// 	{Tag: "kuning", OlseraID: "80487828", Type: "color_tag"},
	// 	{Tag: "hijau", OlseraID: "80487828", Type: "color_tag"},
	// 	{Tag: "small", OlseraID: "80487828", Type: "sku_product"},
	// 	{Tag: "merah", OlseraID: "80487827", Type: "color_tag"},
	// 	{Tag: "biru", OlseraID: "80487827", Type: "color_tag"},
	// 	{Tag: "big", OlseraID: "80487827", Type: "sku_product"},
	// },
	// "Diskonter Bintaro": {
	// 	{Tag: "kuning", OlseraID: "78837902", Type: "color_tag"},
	// 	{Tag: "hijau", OlseraID: "78837902", Type: "color_tag"},
	// 	{Tag: "small", OlseraID: "78837902", Type: "sku_product"},
	// 	{Tag: "merah", OlseraID: "78837901", Type: "color_tag"},
	// 	{Tag: "biru", OlseraID: "78837901", Type: "color_tag"},
	// 	{Tag: "big", OlseraID: "78837901", Type: "sku_product"},
	// },
	// "Diskonter Pekayon": {
	// 	{Tag: "kuning", OlseraID: "78146542", Type: "color_tag"},
	// 	{Tag: "hijau", OlseraID: "78146542", Type: "color_tag"},
	// 	{Tag: "small", OlseraID: "78146542", Type: "sku_product"},
	// 	{Tag: "merah", OlseraID: "78146541", Type: "color_tag"},
	// 	{Tag: "biru", OlseraID: "78146541", Type: "color_tag"},
	// 	{Tag: "big", OlseraID: "78146541", Type: "sku_product"},
	// },
	// "Diskonter Harapan": {
	// 	{Tag: "kuning", OlseraID: "77618277", Type: "color_tag"},
	// 	{Tag: "hijau", OlseraID: "77618277", Type: "color_tag"},
	// 	{Tag: "small", OlseraID: "77618277", Type: "sku_product"},
	// 	{Tag: "merah", OlseraID: "77618278", Type: "color_tag"},
	// 	{Tag: "biru", OlseraID: "77618278", Type: "color_tag"},
	// 	{Tag: "big", OlseraID: "77618278", Type: "sku_product"},
	// },
	// "Diskonter Loji": {
	// 	{Tag: "kuning", OlseraID: "76661552", Type: "color_tag"},
	// 	{Tag: "hijau", OlseraID: "76661552", Type: "color_tag"},
	// 	{Tag: "small", OlseraID: "76661552", Type: "sku_product"},
	// 	{Tag: "merah", OlseraID: "76661551", Type: "color_tag"},
	// 	{Tag: "biru", OlseraID: "76661551", Type: "color_tag"},
	// 	{Tag: "big", OlseraID: "76661551", Type: "sku_product"},
	// },
	// "Diskonter Mayor Oking": {
	// 	{Tag: "kuning", OlseraID: "70286445", Type: "color_tag"},
	// 	{Tag: "hijau", OlseraID: "70286445", Type: "color_tag"},
	// 	{Tag: "small", OlseraID: "70286445", Type: "sku_product"},
	// 	{Tag: "merah", OlseraID: "70286562", Type: "color_tag"},
	// 	{Tag: "biru", OlseraID: "70286562", Type: "color_tag"},
	// 	{Tag: "big", OlseraID: "70286562", Type: "sku_product"},
	// },
}

func GetTagMapping(storeName string) (map[string]string, error) {
	items, ok := DiskonterData[storeName]
	if !ok {
		return nil, fmt.Errorf("Store %s tidak ditemukan di config olsera_mapping", storeName)
	}

	result := make(map[string]string)
	for _, item := range items {
		result[item.Tag] = item.OlseraID
	}

	return result, nil
}