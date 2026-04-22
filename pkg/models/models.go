package models

import "time"

type DeviceType struct {
	Type  string `json:"type"`
	Model string `json:"model"`
	Brand string `json:"brand"`
	OS    string `json:"os"`
}

type DeviceInfo struct {
	OS           string     `json:"os"`
	UserAgent    string     `json:"userAgent"`
	Browser      string     `json:"browser"`
	Product      string     `json:"product"`
	Manufacturer string     `json:"manufacturer"`
	Engine       string     `json:"engine"`
	Fingerprint  string     `json:"fingerprint"`
	Width        int        `json:"width"`
	Height       int        `json:"height"`
	Device       DeviceType `json:"device"`
	PixelRatio   float64    `json:"pixelRatio"`
	DarkMode     bool       `json:"dark_mode"`
	Language     string     `json:"language"`
}

type Session struct {
	ID                string     `json:"id"`
	CreatedAt         time.Time  `json:"created_at"`
	DeviceInfo        DeviceInfo `json:"device_info"`
	BrowserType       string     `json:"browser_type"`
	AccessKey         string     `json:"access_key"`
	LastURL           string     `json:"last_url"`
	WebhookURL        string     `json:"webhook_url"`
	ConnectionState   string     `json:"connection_state"`
	OwnerJID          string     `json:"owner_jid"`
	ProfileName       string     `json:"profile_name"`
	ProfilePictureURL string     `json:"profile_picture_url"`
}

type Message struct {
	ID        string    `json:"id"`
	SessionID string    `json:"session_id"`
	Body      string    `json:"body"`
	Type      string    `json:"type"`
	From      string    `json:"from"`
	To        string    `json:"to"`
	Self      string    `json:"self"` // "in" or "out"
	IsGroup   bool      `json:"is_group"`
	PushName  string    `json:"pushname"`
	Timestamp int64     `json:"timestamp"`
}
