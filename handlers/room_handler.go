package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/base32"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

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

// generateUniqueQRString mencoba beberapa kali membuat qr_string yang belum
// dipakai ruangan lain (kemungkinan tabrakan sangat kecil, tapi tetap dicek).
func generateUniqueQRString() (string, error) {
	for attempt := 0; attempt < 5; attempt++ {
		candidate, err := generateQRString()
		if err != nil {
			return "", err
		}
		var exists int
		config.DB.QueryRow(`SELECT COUNT(*) FROM rooms WHERE qr_string = ?`, candidate).Scan(&exists)
		if exists == 0 {
			return candidate, nil
		}
	}
	return "", sql.ErrNoRows
}

// rotateRoomQR membuat qr_string baru untuk sebuah ruangan dan menyimpannya.
// HANYA dipanggil saat admin menekan tombol "Refresh Sekarang" (manual) —
// QR TIDAK pernah berganti sendiri di background/otomatis, supaya QR yang
// sudah dicetak/ditempel tetap valid sampai admin sengaja me-refresh-nya.
func rotateRoomQR(roomID uint64) (string, time.Time, error) {
	qr, err := generateUniqueQRString()
	if err != nil {
		return "", time.Time{}, err
	}
	rotatedAt := time.Now()

	_, err = config.DB.Exec(
		`UPDATE rooms SET qr_string = ?, qr_last_rotated_at = ? WHERE id = ?`,
		qr, rotatedAt, roomID,
	)
	if err != nil {
		return "", time.Time{}, err
	}
	return qr, rotatedAt, nil
}

// GET /api/admin/rooms
func ListRooms(w http.ResponseWriter, r *http.Request) {
	rows, err := config.DB.Query(`SELECT id, name, qr_string, is_active, qr_last_rotated_at FROM rooms ORDER BY name ASC`)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil data ruangan: "+err.Error())
		return
	}
	defer rows.Close()

	var rooms []models.Room
	for rows.Next() {
		var rm models.Room
		var lastRotatedAt sql.NullTime
		if err := rows.Scan(&rm.ID, &rm.Name, &rm.QRString, &rm.IsActive, &lastRotatedAt); err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca data: "+err.Error())
			return
		}
		if lastRotatedAt.Valid {
			t := lastRotatedAt.Time
			rm.QRLastRotatedAt = &t
		}
		rooms = append(rooms, rm)
	}

	utils.Success(w, http.StatusOK, "Berhasil mengambil data ruangan", rooms)
}

// POST /api/admin/rooms
// Membuat ruangan baru sekaligus generate qr_string unik pertama kalinya.
// Setelah ini QR ruangan tidak akan berubah lagi kecuali admin menekan
// tombol refresh manual di halaman QR Live.
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

	qr, err := generateUniqueQRString()
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal membuat QR string unik, coba lagi")
		return
	}

	result, err := config.DB.Exec(
		`INSERT INTO rooms (name, qr_string, qr_last_rotated_at) VALUES (?, ?, NOW())`,
		req.Name, qr,
	)
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

// =====================================================================
// GET /api/admin/rooms/{id}/qr
// Mengambil qr_string ruangan APA ADANYA (tanpa efek samping merotasi).
// Aman dipanggil berkali-kali / dibuka ulang -- QR yang ditampilkan/dicetak
// akan selalu sama sampai admin menekan tombol "Refresh Sekarang".
// =====================================================================
func GetRoomQR(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}

	var name, qr string
	var isActive bool
	var lastRotatedAt sql.NullTime
	err = config.DB.QueryRow(`SELECT name, qr_string, is_active, qr_last_rotated_at FROM rooms WHERE id = ?`, id).
		Scan(&name, &qr, &isActive, &lastRotatedAt)
	if err == sql.ErrNoRows {
		utils.Error(w, http.StatusNotFound, "Ruangan tidak ditemukan")
		return
	} else if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil QR ruangan: "+err.Error())
		return
	}

	resp := map[string]interface{}{
		"id":        id,
		"name":      name,
		"qr_string": qr,
		"is_active": isActive,
	}
	if lastRotatedAt.Valid {
		resp["last_rotated_at"] = lastRotatedAt.Time.Format(time.RFC3339)
	}

	utils.Success(w, http.StatusOK, "Berhasil mengambil QR ruangan", resp)
}

// =====================================================================
// POST /api/admin/rooms/{id}/refresh-qr
// Satu-satunya cara qr_string ruangan berubah: admin menekan tombol ini
// secara sengaja (misalnya karena QR lama dicurigai difoto/tersebar, atau
// stiker lama rusak dan perlu dicetak ulang). Tidak ada rotasi otomatis
// di background maupun saat halaman dibuka/di-refresh biasa.
// =====================================================================
func RefreshRoomQR(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}

	var name string
	var isActive bool
	err = config.DB.QueryRow(`SELECT name, is_active FROM rooms WHERE id = ?`, id).Scan(&name, &isActive)
	if err == sql.ErrNoRows {
		utils.Error(w, http.StatusNotFound, "Ruangan tidak ditemukan")
		return
	} else if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil data ruangan: "+err.Error())
		return
	}

	qr, rotatedAt, err := rotateRoomQR(id)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal me-refresh QR ruangan: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "QR Code berhasil di-refresh", map[string]interface{}{
		"id":              id,
		"name":            name,
		"qr_string":       qr,
		"is_active":       isActive,
		"last_rotated_at": rotatedAt.Format(time.RFC3339),
	})
}
