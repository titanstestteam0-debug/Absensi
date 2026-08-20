package routes

import (
	"net/http"

	"absensi-backend/handlers"
	"absensi-backend/middleware"

	"github.com/gorilla/mux"
)

func SetupRouter() *mux.Router {
	r := mux.NewRouter()
	api := r.PathPrefix("/api").Subrouter()

	// -------------------- Public --------------------
	api.HandleFunc("/auth/login", handlers.Login).Methods(http.MethodPost)

	// -------------------- Profil (butuh login, semua role) --------------------
	// Setiap user yang login (admin, guru, guru_pengganti) bisa lihat & update
	// foto profilnya sendiri lewat endpoint ini.
	me := api.PathPrefix("").Subrouter()
	me.Use(middleware.JWTAuth)
	me.Use(middleware.RequireAnyRole("admin", "guru", "guru_pengganti"))

	me.HandleFunc("/profile", handlers.GetProfile).Methods(http.MethodGet)
	me.HandleFunc("/profile/photo", handlers.UpdateProfilePhoto).Methods(http.MethodPut)

	// -------------------- Guru & Guru Pengganti (butuh login) --------------------
	// Guru Pengganti (Inval) memakai endpoint yang sama persis dengan Guru
	// utama untuk presensi (dengan substitute_for_id) dan pengajuan cuti,
	// jadi kedua role diizinkan lewat RequireAnyRole di sini.
	guru := api.PathPrefix("").Subrouter()
	guru.Use(middleware.JWTAuth)
	guru.Use(middleware.RequireAnyRole("guru", "guru_pengganti"))

	guru.HandleFunc("/attendance/scan-in", handlers.ScanIn).Methods(http.MethodPost)
	guru.HandleFunc("/attendance/scan-out", handlers.ScanOut).Methods(http.MethodPost)
	guru.HandleFunc("/attendance/history", handlers.MyAttendanceHistory).Methods(http.MethodGet)

	guru.HandleFunc("/leaves", handlers.CreateLeave).Methods(http.MethodPost)
	guru.HandleFunc("/leaves", handlers.ListMyLeaves).Methods(http.MethodGet)
	guru.HandleFunc("/teachers", handlers.ListTeachers).Methods(http.MethodGet)

	// -------------------- Admin (butuh login + role admin) --------------------
	admin := api.PathPrefix("/admin").Subrouter()
	admin.Use(middleware.JWTAuth)
	admin.Use(middleware.RequireRole("admin"))

	// Master data guru
	admin.HandleFunc("/teachers", handlers.ListTeachers).Methods(http.MethodGet)
	admin.HandleFunc("/teachers", handlers.CreateTeacher).Methods(http.MethodPost)
	admin.HandleFunc("/teachers/{id}", handlers.UpdateTeacher).Methods(http.MethodPut)
	admin.HandleFunc("/teachers/{id}", handlers.DeleteTeacher).Methods(http.MethodDelete)
	admin.HandleFunc("/teachers/{id}/activate", handlers.ActivateTeacher).Methods(http.MethodPut)

	// Master data ruangan + generator QR
	admin.HandleFunc("/rooms", handlers.ListRooms).Methods(http.MethodGet)
	admin.HandleFunc("/rooms", handlers.CreateRoom).Methods(http.MethodPost)
	admin.HandleFunc("/rooms/{id}", handlers.UpdateRoom).Methods(http.MethodPut)
	admin.HandleFunc("/rooms/{id}", handlers.DeleteRoom).Methods(http.MethodDelete)
	admin.HandleFunc("/rooms/{id}/qr", handlers.GetRoomQR).Methods(http.MethodGet)
	admin.HandleFunc("/rooms/{id}/refresh-qr", handlers.RefreshRoomQR).Methods(http.MethodPost)

	// Jadwal
	admin.HandleFunc("/schedules", handlers.ListSchedules).Methods(http.MethodGet)
	admin.HandleFunc("/schedules", handlers.CreateSchedule).Methods(http.MethodPost)
	admin.HandleFunc("/schedules/{id}", handlers.UpdateSchedule).Methods(http.MethodPut)
	admin.HandleFunc("/schedules/{id}", handlers.DeleteSchedule).Methods(http.MethodDelete)

	// Cuti/izin
	admin.HandleFunc("/leaves", handlers.ListAllLeaves).Methods(http.MethodGet)
	admin.HandleFunc("/leaves/{id}/approve", handlers.ApproveLeave).Methods(http.MethodPut)
	admin.HandleFunc("/leaves/{id}/reject", handlers.RejectLeave).Methods(http.MethodPut)

	// Laporan
	admin.HandleFunc("/reports/monthly", handlers.MonthlyReport).Methods(http.MethodGet)
	admin.HandleFunc("/reports/history", handlers.HistoryLog).Methods(http.MethodGet)

	return r
}
