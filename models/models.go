package models

import "time"

type Role string

const (
	RoleAdmin         Role = "admin"
	RoleGuru          Role = "guru"           // guru utama, punya jadwal tetap
	RoleGuruPengganti Role = "guru_pengganti" // guru inval, presensi menggantikan guru utama yang cuti
)

type User struct {
	ID           uint64    `json:"id"`
	Name         string    `json:"name"`
	NIP          string    `json:"nip"`
	Email        string    `json:"email,omitempty"`
	PhotoURL     string    `json:"photo_url,omitempty"`
	PasswordHash string    `json:"-"`
	Role         Role      `json:"role"`
	IsActive     bool      `json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
}

type Room struct {
	ID          uint64     `json:"id"`
	Name        string     `json:"name"`
	QRString    string     `json:"qr_string"`
	IsActive    bool       `json:"is_active"`
	QRExpiresAt *time.Time `json:"qr_expires_at,omitempty"`
}

type Schedule struct {
	ID          uint64 `json:"id"`
	TeacherID   uint64 `json:"teacher_id"`
	TeacherName string `json:"teacher_name,omitempty"`
	RoomID      uint64 `json:"room_id"`
	RoomName    string `json:"room_name,omitempty"`
	DayOfWeek   int    `json:"day_of_week"` // 1=Senin ... 7=Minggu
	StartTime   string `json:"start_time"`  // "HH:MM:SS"
	EndTime     string `json:"end_time"`
	TargetJP    int    `json:"target_jp"`
	Subject     string `json:"subject,omitempty"`
	IsActive    bool   `json:"is_active"`
}

type LeaveStatus string

const (
	LeavePending  LeaveStatus = "pending"
	LeaveApproved LeaveStatus = "approved"
	LeaveRejected LeaveStatus = "rejected"
)

type Leave struct {
	ID              uint64      `json:"id"`
	TeacherID       uint64      `json:"teacher_id"`
	StartDate       string      `json:"start_date"` // "YYYY-MM-DD"
	EndDate         string      `json:"end_date"`
	LeaveType       string      `json:"leave_type"`
	Reason          string      `json:"reason"`
	AttachmentURL   string      `json:"attachment_url,omitempty"`
	Status          LeaveStatus `json:"status"`
	RejectionReason string      `json:"rejection_reason,omitempty"`
	ApprovedBy      *uint64     `json:"approved_by,omitempty"`
	CreatedAt       time.Time   `json:"created_at"`
}

type AttendanceStatus string

const (
	AttInProgress  AttendanceStatus = "in_progress"
	AttTuntas      AttendanceStatus = "tuntas"
	AttTidakTuntas AttendanceStatus = "tidak_tuntas"
	AttTidakHadir  AttendanceStatus = "tidak_hadir"
)

type Attendance struct {
	ID                  uint64           `json:"id"`
	ScheduleID          uint64           `json:"schedule_id"`
	TeacherID           uint64           `json:"teacher_id"`
	SubstituteTeacherID *uint64          `json:"substitute_teacher_id,omitempty"`
	Date                string           `json:"date"`
	ClockIn             *time.Time       `json:"clock_in,omitempty"`
	ClockOut            *time.Time       `json:"clock_out,omitempty"`
	ActualJP            float64          `json:"actual_jp"`
	Status              AttendanceStatus `json:"status"`
	RoomIDScanned       *uint64          `json:"room_id_scanned,omitempty"`
}
