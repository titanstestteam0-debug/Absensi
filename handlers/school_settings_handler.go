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
	Tagline     string `json:"tagline"`
}

// Batas ukuran logo (dalam karakter data URL base64) supaya tabel tidak
// membengkak dan payload tetap wajar untuk dikirim di tiap request header.
// ~2.5 juta karakter base64 kurang lebih setara logo ~1.8MB asli -- lebih
// dari cukup untuk logo sekolah, sekaligus jadi guardrail.
const maxLogoDataURLLength = 2_500_000

// Subjudul header dibatasi supaya tidak merusak tata letak header (yang
// sempit di layar mobile).
const maxTaglineLength = 150

// GET /api/settings/school
// PUBLIC (tanpa login) -- dipakai buat render header & modal login
// SEBELUM user login, jadi tidak boleh diwajibkan JWT.
func GetSchoolSettings(w http.ResponseWriter, r *http.Request) {
	var s models.SchoolSettings
	var name, logo, tagline sql.NullString
	err := config.DB.QueryRow(`SELECT school_name, logo_data_url, tagline FROM school_settings WHERE id = 1`).
		Scan(&name, &logo, &tagline)
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
	if tagline.Valid {
		s.Tagline = &tagline.String
	}

	utils.Success(w, http.StatusOK, "Berhasil mengambil identitas sekolah", s)
}

// PUT /api/admin/settings/school
// Body: { "school_name": "SMA Negeri 1 Contoh", "logo_data_url": "data:image/png;base64,...", "tagline": "Teks di bawah nama sekolah" }
// Ketiganya opsional -- kirim string kosong untuk mengembalikan ke default
// bawaan aplikasi ("SIM-ABSENSI GURU" + ikon 🏫 + "Sistem Presensi Mengajar berbasis QR Code").
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
	if len(req.Tagline) > maxTaglineLength {
		utils.Error(w, http.StatusBadRequest, "Teks subjudul terlalu panjang, maksimal 150 karakter")
		return
	}

	var namePtr, logoPtr, taglinePtr interface{}
	if req.SchoolName != "" {
		namePtr = req.SchoolName
	}
	if req.LogoDataURL != "" {
		logoPtr = req.LogoDataURL
	}
	if req.Tagline != "" {
		taglinePtr = req.Tagline
	}

	_, err := config.DB.Exec(
		`INSERT INTO school_settings (id, school_name, logo_data_url, tagline) VALUES (1, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE school_name = VALUES(school_name), logo_data_url = VALUES(logo_data_url), tagline = VALUES(tagline)`,
		namePtr, logoPtr, taglinePtr,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal menyimpan identitas sekolah: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "Identitas sekolah berhasil disimpan", nil)
}
