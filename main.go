package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	// Import blank ini menyertakan database timezone IANA langsung di dalam
	// binary. Tanpa ini, time.LoadLocation("Asia/Jakarta") bisa gagal di
	// container/hosting yang tidak menyediakan file tzdata OS (mis. image
	// minimal ala Railway/Nixpacks), karena Go butuh sumber data zona waktu.
	_ "time/tzdata"

	"absensi-backend/config"
	"absensi-backend/routes"

	"github.com/gorilla/handlers"
	"github.com/joho/godotenv"
)

func main() {
	// Muat file .env kalau ada (di production biasanya env var di-set langsung
	// lewat sistem, jadi tidak masalah kalau .env tidak ditemukan).
	if err := godotenv.Load(); err != nil {
		log.Println("info: .env tidak ditemukan, pakai environment variable yang sudah ada")
	}

	// PENTING: kunci timezone aplikasi ke Asia/Jakarta (WIB), jangan andalkan
	// timezone bawaan OS server. Semua pemanggilan time.Now() di aplikasi ini
	// (termasuk validasi jam scan-in/out) dan koneksi database (yang pakai
	// DSN "loc=Local") sama-sama mengikuti time.Local, jadi cukup di-set
	// sekali di sini supaya konsisten di mana pun di-deploy (lokal, Railway,
	// TiDB Cloud, dll) - tidak lagi bergantung timezone container hosting.
	jakarta, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		log.Fatalf("gagal memuat timezone Asia/Jakarta: %v", err)
	}
	time.Local = jakarta

	config.ConnectDB()
	defer config.DB.Close()

	router := routes.SetupRouter()

	// Railway (dan platform hosting lain) inject PORT secara otomatis dan
	// mengharuskan app listen di port itu. Kalau tidak ada (misal jalan lokal),
	// fallback ke APP_PORT lalu default 8080.
	port := os.Getenv("PORT")
	if port == "" {
		port = os.Getenv("APP_PORT")
	}
	if port == "" {
		port = "8080"
	}

	// CORS_ALLOWED_ORIGINS: daftar origin frontend yang boleh akses API,
	// dipisah koma. Contoh: https://absensi-guru.vercel.app,http://localhost:5173
	// Default ke localhost:5173 kalau env var tidak di-set (buat dev lokal).
	allowedOrigins := []string{"http://localhost:5173"}
	if raw := os.Getenv("CORS_ALLOWED_ORIGINS"); raw != "" {
		allowedOrigins = strings.Split(raw, ",")
		for i := range allowedOrigins {
			allowedOrigins[i] = strings.TrimSpace(allowedOrigins[i])
		}
	}

	corsRouter := handlers.CORS(
		handlers.AllowedOrigins(allowedOrigins),
		handlers.AllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
		handlers.AllowedHeaders([]string{"Content-Type", "Authorization"}),
	)(router)

	log.Printf("Server berjalan di port %s\n", port)
	if err := http.ListenAndServe(":"+port, corsRouter); err != nil {
		log.Fatalf("Gagal menjalankan server: %v", err)
	}
}
