package config

import (
	"crypto/tls"
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/go-sql-driver/mysql"
)

var DB *sql.DB

// ConnectDB membuka koneksi ke MySQL (atau TiDB Cloud, yang wire-compatible
// dengan MySQL) menggunakan environment variables:
// DB_USER, DB_PASSWORD, DB_HOST, DB_PORT, DB_NAME, DB_TLS_MODE
//
// DB_TLS_MODE:
//   - "true"        -> TLS wajib, verifikasi sertifikat pakai root CA sistem
//                       (INI YANG DIPAKAI TIDB CLOUD SERVERLESS)
//   - "skip-verify"  -> TLS aktif tapi sertifikat tidak diverifikasi (hanya untuk debug)
//   - "false"/kosong -> tanpa TLS (dipakai untuk MySQL lokal / XAMPP)
func ConnectDB() {
	user := getEnv("DB_USER", "root")
	pass := getEnv("DB_PASSWORD", "")
	host := getEnv("DB_HOST", "127.0.0.1")
	port := getEnv("DB_PORT", "3306")
	name := getEnv("DB_NAME", "absensi_guru")
	tlsMode := getEnv("DB_TLS_MODE", "false")

	tlsParam := ""
	switch tlsMode {
	case "true":
		// TiDB Cloud Serverless memakai sertifikat dari CA publik (Let's Encrypt/
		// DigiCert), jadi cukup andalkan root CA bawaan sistem operasi -
		// tidak perlu upload file CA manual.
		if err := mysql.RegisterTLSConfig("tidb", &tls.Config{MinVersion: tls.VersionTLS12}); err != nil {
			log.Fatalf("gagal register TLS config: %v", err)
		}
		tlsParam = "&tls=tidb"
	case "skip-verify":
		tlsParam = "&tls=skip-verify"
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&loc=Local&charset=utf8mb4%s",
		user, pass, host, port, name, tlsParam)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("gagal membuka koneksi database: %v", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(25)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		log.Fatalf("gagal konek ke database: %v", err)
	}

	log.Println("berhasil konek ke database MySQL:", name)
	DB = db
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
