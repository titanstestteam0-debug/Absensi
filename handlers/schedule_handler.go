package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"absensi-backend/config"
	"absensi-backend/models"
	"absensi-backend/utils"

	"github.com/gorilla/mux"
)

type ScheduleRequest struct {
	TeacherID uint64 `json:"teacher_id"`
	RoomID    uint64 `json:"room_id"`
	DayOfWeek int    `json:"day_of_week"` // 1=Senin ... 7=Minggu
	StartTime string `json:"start_time"`  // "HH:MM" atau "HH:MM:SS"
	EndTime   string `json:"end_time"`
	TargetJP  int    `json:"target_jp"`
	Subject   string `json:"subject"`
}

// GET /api/admin/schedules
func ListSchedules(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT s.id, s.teacher_id, u.name, s.room_id, rm.name, s.day_of_week,
		       s.start_time, s.end_time, s.target_jp, IFNULL(s.subject, ''), s.is_active
		FROM schedules s
		JOIN users u ON u.id = s.teacher_id
		JOIN rooms rm ON rm.id = s.room_id
		ORDER BY s.day_of_week ASC, s.start_time ASC`

	rows, err := config.DB.Query(query)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal mengambil jadwal: "+err.Error())
		return
	}
	defer rows.Close()

	var schedules []models.Schedule
	for rows.Next() {
		var s models.Schedule
		if err := rows.Scan(&s.ID, &s.TeacherID, &s.TeacherName, &s.RoomID, &s.RoomName,
			&s.DayOfWeek, &s.StartTime, &s.EndTime, &s.TargetJP, &s.Subject, &s.IsActive); err != nil {
			utils.Error(w, http.StatusInternalServerError, "Gagal membaca data: "+err.Error())
			return
		}
		schedules = append(schedules, s)
	}

	utils.Success(w, http.StatusOK, "Berhasil mengambil jadwal", schedules)
}

// POST /api/admin/schedules
func CreateSchedule(w http.ResponseWriter, r *http.Request) {
	var req ScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Error(w, http.StatusBadRequest, "Body request tidak valid")
		return
	}
	if req.TeacherID == 0 || req.RoomID == 0 || req.DayOfWeek < 1 || req.DayOfWeek > 7 ||
		req.StartTime == "" || req.EndTime == "" || req.TargetJP <= 0 {
		utils.Error(w, http.StatusBadRequest, "Semua field wajib diisi dengan benar")
		return
	}

	result, err := config.DB.Exec(
		`INSERT INTO schedules (teacher_id, room_id, day_of_week, start_time, end_time, target_jp, subject)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		req.TeacherID, req.RoomID, req.DayOfWeek, req.StartTime, req.EndTime, req.TargetJP, req.Subject,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal membuat jadwal: "+err.Error())
		return
	}

	id, _ := result.LastInsertId()
	utils.Success(w, http.StatusCreated, "Jadwal berhasil dibuat", map[string]interface{}{"id": id})
}

// PUT /api/admin/schedules/{id}
func UpdateSchedule(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}

	var req ScheduleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.Error(w, http.StatusBadRequest, "Body request tidak valid")
		return
	}

	_, err = config.DB.Exec(
		`UPDATE schedules SET teacher_id=?, room_id=?, day_of_week=?, start_time=?, end_time=?, target_jp=?, subject=?
		 WHERE id=?`,
		req.TeacherID, req.RoomID, req.DayOfWeek, req.StartTime, req.EndTime, req.TargetJP, req.Subject, id,
	)
	if err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal memperbarui jadwal: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "Jadwal berhasil diperbarui", nil)
}

// DELETE /api/admin/schedules/{id}
func DeleteSchedule(w http.ResponseWriter, r *http.Request) {
	idStr := mux.Vars(r)["id"]
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		utils.Error(w, http.StatusBadRequest, "ID tidak valid")
		return
	}

	if _, err := config.DB.Exec(`UPDATE schedules SET is_active = 0 WHERE id = ?`, id); err != nil {
		utils.Error(w, http.StatusInternalServerError, "Gagal menonaktifkan jadwal: "+err.Error())
		return
	}

	utils.Success(w, http.StatusOK, "Jadwal berhasil dinonaktifkan", nil)
}
