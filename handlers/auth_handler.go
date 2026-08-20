package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"absensi-backend/config"
	"absensi-backend/utils"

	"golang.org/x/crypto/bcrypt"
)

type LoginRequest struct {
	NIP      string `json:"nip"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string      `json:"token"`
	User  interface{} `json:"user"`
}

// POST /api/auth/login
// Login menggunakan NIP + password, mengembalikan JWT.
func Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Error(w, http.StatusBadRequest, "Body request tidak valid")
		return
	}
	if req.NIP == "" || req.Password == "" {
		utils.Error(w, http.StatusBadRequest, "NIP dan password wajib diisi")
		return
	}

	var (
		id           uint64
		name         string
		email        sql.NullString
		photoURL     sql.NullString
		passwordHash string
		role         string
		isActive     bool
	)

	query := `SELECT id, name, email, photo_url, password_hash, role, is_active
	          FROM users WHERE nip = ? LIMIT 1`
	err := config.DB.QueryRow(query, req.NIP).Scan(&id, &name, &email, &photoURL, &passwordHash, &role, &isActive)
	if err == sql.ErrNoRows {
		utils.Error(w, http.StatusUnauthorized, "NIP atau password salah")
		return
	} else if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal memproses login: "+err.Error())
		return
	}

	if !isActive {
		utils.Error(w, http.StatusForbidden, "Akun Anda tidak aktif, hubungi admin")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		utils.Error(w, http.StatusUnauthorized, "NIP atau password salah")
		return
	}

	token, err := utils.GenerateToken(id, role)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal membuat token")
		return
	}

	utils.Success(w, http.StatusOK, "Login berhasil", LoginResponse{
		Token: token,
		User: map[string]interface{}{
			"id":        id,
			"name":      name,
			"email":     email.String,
			"photo_url": photoURL.String,
			"role":      role,
		},
	})
}
