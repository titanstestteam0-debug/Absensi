package handlers

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"

	"absensi-backend/config"
	"absensi-backend/middleware"
	"absensi-backend/utils"
)

// Ukuran maksimal foto profil setelah didekode dari base64. Dibatasi supaya
// kolom LONGTEXT di database & payload JSON tidak membengkak tanpa batas.
const maxPhotoBytes = 2 * 1024 * 1024 // 2 MB

var allowedPhotoMimeTypes = map[string]bool{
	"image/jpeg": true,
	"image/jpg":  true,
	"image/png":  true,
	"image/webp": true,
}

// GET /api/profile
// Setiap user yang login (admin, guru, guru_pengganti) bisa melihat profilnya sendiri.
func GetProfile(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var (
		id       uint64
		name     string
		nip      string
		email    sql.NullString
		photoURL sql.NullString
		role     string
		isActive bool
	)

	err := config.DB.QueryRow(
		`SELECT id, name, nip, email, photo_url, role, is_active FROM users WHERE id = ? LIMIT 1`,
		userID,
	).Scan(&id, &name, &nip, &email, &photoURL, &role, &isActive)
	if err == sql.ErrNoRows {
		utils.Error(w, http.StatusNotFound, "Profil tidak ditemukan")
		return
	} else if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil profil: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "Berhasil mengambil profil", map[string]interface{}{
		"id":        id,
		"name":      name,
		"nip":       nip,
		"email":     email.String,
		"photo_url": photoURL.String,
		"role":      role,
		"is_active": isActive,
	})
}

type UpdatePhotoRequest struct {
	// PhotoBase64 berupa data URI lengkap, mis:
	// "data:image/jpeg;base64,/9j/4AAQSkZJRg..."
	PhotoBase64 string `json:"photo_base64"`
}

// PUT /api/profile/photo
// Guru (atau siapa pun yang login) memperbarui foto profilnya sendiri.
// Foto dikirim sebagai data URI base64 dan disimpan langsung di kolom
// users.photo_url (bukan disk), supaya tetap ada meski aplikasi di-redeploy
// di hosting dengan filesystem yang tidak persisten (mis. Railway).
func UpdateProfilePhoto(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)

	var req UpdatePhotoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Error(w, http.StatusBadRequest, "Body request tidak valid")
		return
	}

	dataURI := strings.TrimSpace(req.PhotoBase64)
	if dataURI == "" {
		utils.Error(w, http.StatusBadRequest, "photo_base64 wajib diisi")
		return
	}

	if !strings.HasPrefix(dataURI, "data:") {
		utils.Error(w, http.StatusBadRequest, "Format foto tidak valid, harus berupa data URI base64")
		return
	}

	commaIdx := strings.Index(dataURI, ",")
	if commaIdx == -1 {
		utils.Error(w, http.StatusBadRequest, "Format foto tidak valid")
		return
	}
	header := dataURI[len("data:"):commaIdx] // contoh: "image/jpeg;base64"
	meta := strings.Split(header, ";")
	mimeType := meta[0]

	if !allowedPhotoMimeTypes[strings.ToLower(mimeType)] {
		utils.Error(w, http.StatusBadRequest, "Tipe file harus JPEG, PNG, atau WEBP")
		return
	}

	payload := dataURI[commaIdx+1:]
	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "Data foto tidak valid (base64 gagal didekode)")
		return
	}
	if len(decoded) > maxPhotoBytes {
		utils.Error(w, http.StatusBadRequest, "Ukuran foto maksimal 2MB")
		return
	}
	if len(decoded) == 0 {
		utils.Error(w, http.StatusBadRequest, "Data foto kosong")
		return
	}

	if _, err := config.DB.Exec(`UPDATE users SET photo_url = ? WHERE id = ?`, dataURI, userID); err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal menyimpan foto profil: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "Foto profil berhasil diperbarui", map[string]interface{}{
		"photo_url": dataURI,
	})
}
