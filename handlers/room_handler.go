package handlers

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"absensi-backend/config"
	"absensi-backend/models"
	"absensi-backend/utils"

	"github.com/gorilla/mux"
)

type RoomRequest struct {
	Name string `json:"name"`
}

// generateQRString membuat unique hash pendek untuk QR Code ruangan.
// Contoh hasil: ROOM-K3F9X2P7
func generateQRString() (string, error) {
	buf := make([]byte, 5)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	code := strings.ToUpper(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf))
	return "ROOM-" + code, nil
}

// GET /api/admin/rooms
func ListRooms(w http.ResponseWriter, r *http.Request) {
	rows, err := config.DB.Query(`SELECT id, name, qr_string, is_active FROM rooms ORDER BY name ASC`)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil data ruangan: "+err.Error())
		return
	}
	defer rows.Close()

	var rooms []models.Room
	for rows.Next() {
		var rm models.Room
		if err := rows.Scan(&rm.ID, &rm.Name, &rm.QRString, &rm.IsActive); err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca data: "+err.Error())
			return
		}
		rooms = append(rooms, rm)
	}

	utils.Success(w, http.StatusOK, "Berhasil mengambil data ruangan", rooms)
}

// POST /api/admin/rooms
// Membuat ruangan baru sekaligus generate qr_string unik untuk dicetak.
func CreateRoom(w http.ResponseWriter, r *http.Request) {
	var req RoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Error(w, http.StatusBadRequest, "Body request tidak valid")
		return
	}
	if req.Name == "" {
		utils.Error(w, http.StatusBadRequest, "name wajib diisi")
		return
	}

	var qr string
	// coba beberapa kali jika terjadi tabrakan (kemungkinan sangat kecil)
	for attempt := 0; attempt < 5; attempt++ {
		candidate, err := generateQRString()
		if err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal membuat QR string")
			return
		}
		var exists int
		config.DB.QueryRow(`SELECT COUNT(*) FROM rooms WHERE qr_string = ?`, candidate).Scan(&exists)
		if exists == 0 {
			qr = candidate
			break
		}
	}
	if qr == "" {
		utils.Error(w, http.StatusInternalServerError, "Gagal membuat QR string unik, coba lagi")
		return
	}

	result, err := config.DB.Exec(`INSERT INTO rooms (name, qr_string) VALUES (?, ?)`, req.Name, qr)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal membuat ruangan: "+err.Error())
		return
	}

	id, _ := result.LastInsertId()
	utils.Success(w, http.StatusCreated, "Ruangan berhasil dibuat", map[string]interface{}{
		"id":        id,
		"name":      req.Name,
		"qr_string": qr,
	})
}

// PUT /api/admin/rooms/{id}
func UpdateRoom(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}

	var req RoomRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Error(w, http.StatusBadRequest, "Body request tidak valid")
		return
	}

	if _, err := config.DB.Exec(`UPDATE rooms SET name = ? WHERE id = ?`, req.Name, id); err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal memperbarui ruangan: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "Ruangan berhasil diperbarui", nil)
}

// DELETE /api/admin/rooms/{id}
func DeleteRoom(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}

	if _, err := config.DB.Exec(`UPDATE rooms SET is_active = 0 WHERE id = ?`, id); err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal menonaktifkan ruangan: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "Ruangan berhasil dinonaktifkan", nil)
}
