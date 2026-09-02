package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"absensi-backend/config"
	"absensi-backend/models"
	"absensi-backend/utils"
)

type SchoolSettingsRequest struct {
	SchoolName  string `json:"school_name"`
	LogoDataURL string `json:"logo_data_url"`
}

// Batas ukuran logo (dalam karakter data URL base64) supaya tabel tidak
// membengkak dan payload tetap wajar untuk dikirim di tiap request header.
// ~2.5 juta karakter base64 kurang lebih setara logo ~1.8MB asli -- lebih
// dari cukup untuk logo sekolah, sekaligus jadi guardrail.
const maxLogoDataURLLength = 2_500_000

// GET /api/settings/school
// PUBLIC (tanpa login) -- dipakai buat render header & modal login
// SEBELUM user login, jadi tidak boleh diwajibkan JWT.
func GetSchoolSettings(w http.ResponseWriter, r *http.Request) {
	var s models.SchoolSettings
	var name, logo sql.NullString
	err := config.DB.QueryRow(`SELECT school_name, logo_data_url FROM school_settings WHERE id = 1`).
		Scan(&name, &logo)
	if err != nil && err != sql.ErrNoRows {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil identitas sekolah: "+err.Error())
		return
	}
	if name.Valid {
		s.SchoolName = &name.String
	}
	if logo.Valid {
		s.LogoDataURL = &logo.String
	}

	utils.Success(w, http.StatusOK, "Berhasil mengambil identitas sekolah", s)
}

// PUT /api/admin/settings/school
// Body: { "school_name": "SMA Negeri 1 Contoh", "logo_data_url": "data:image/png;base64,..." }
// Keduanya opsional -- kirim string kosong untuk mengembalikan ke default
// bawaan aplikasi ("SIM-ABSENSI GURU" + ikon 🏫).
func UpdateSchoolSettings(w http.ResponseWriter, r *http.Request) {
	var req SchoolSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Error(w, http.StatusBadRequest, "Body request tidak valid")
		return
	}
	if len(req.LogoDataURL) > maxLogoDataURLLength {
		utils.Error(w, http.StatusBadRequest, "Ukuran logo terlalu besar, gunakan gambar yang lebih kecil/terkompresi")
		return
	}

	var namePtr, logoPtr interface{}
	if req.SchoolName != "" {
		namePtr = req.SchoolName
	}
	if req.LogoDataURL != "" {
		logoPtr = req.LogoDataURL
	}

	_, err := config.DB.Exec(
		`INSERT INTO school_settings (id, school_name, logo_data_url) VALUES (1, ?, ?)
		 ON DUPLICATE KEY UPDATE school_name = VALUES(school_name), logo_data_url = VALUES(logo_data_url)`,
		namePtr, logoPtr,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal menyimpan identitas sekolah: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "Identitas sekolah berhasil disimpan", nil)
}
