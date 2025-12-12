package models

import (
    "database/sql/driver"
    "fmt"
    "time"
    "gorm.io/gorm"
)

type Date time.Time

// MarshalJSON: Mengubah tipe Date menjadi string JSON dengan format "YYYY-MM-DD"
func (d Date) MarshalJSON() ([]byte, error) {
    // Jika waktu bernilai zero value, kembalikan null atau string kosong sesuai kebutuhan
    if time.Time(d).IsZero() {
        return []byte("null"), nil
    }
    // Format standar Go untuk tanggal saja: "2006-01-02"
    return []byte(fmt.Sprintf(`"%s"`, time.Time(d).Format("2006-01-02"))), nil 
}

// UnmarshalJSON: Menganalisis string JSON "YYYY-MM-DD" kembali ke tipe Date
func (d *Date) UnmarshalJSON(data []byte) error {
    s := string(data)
    if s == "null" || s == "" {
        *d = Date{}
        return nil
    }
    // Parse tanggal dengan format "YYYY-MM-DD" (hapus kutip dari string data)
    t, err := time.Parse(`"2006-01-02"`, s) 
    if err != nil {
        return err
    }
    *d = Date(t)
    return nil
}

// Value: Mengimplementasikan driver.Valuer agar GORM tahu cara mengirimnya ke DB
func (d Date) Value() (driver.Value, error) {
    if time.Time(d).IsZero() {
        return nil, nil
    }
    // Mengembalikan tanggal dalam format string yang dikenali database tipe DATE
    return time.Time(d).Format("2006-01-02"), nil
}

// Scan: Mengimplementasikan sql.Scanner agar GORM tahu cara mengambil dari DB
func (d *Date) Scan(value interface{}) error {
    if value == nil {
        *d = Date{}
        return nil
    }
    
    // Database mengembalikan time.Time. Kita potong (truncate) komponen waktunya.
    if t, ok := value.(time.Time); ok {
        *d = Date(t.Truncate(24 * time.Hour)) 
        return nil
    }
    return fmt.Errorf("failed to scan Date type from database: %v", value)
}

type UserScanWeb struct {
	ID         uint			 `gorm:"primaryKey;autoIncrement" json:"id"`
	CodeDocument *string		 `json:"code_document"`
	UserID     *uint		 `json:"user_id"`
	TotalScans *int			 `gorm:"type:integer" json:"total_scan"`
	ScanDate   *Date	 `gorm:"type:date" json:"scan_date"`
	DeletedAt  gorm.DeletedAt `json:"deleted_at"`
	CreatedAt  time.Time	 `json:"created_at"`
	UpdatedAt  time.Time	 `json:"updated_at"`

	Document *Document `gorm:"foreignKey:CodeDocument;references:Code"`
	User     *User     `gorm:"foreignKey:UserID"`
}

// Nama tabel custom
// akan di panggil otmatis
// func (UserScanWeb) TableName() string {
// 	return "user_scan_webs"
// }