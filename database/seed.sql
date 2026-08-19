USE absensi_guru;

-- Password untuk semua akun di bawah ini adalah: password123
-- (hash dibuat dengan bcrypt cost 10 — hanya untuk keperluan testing/demo)
INSERT INTO users (name, nip, email, password_hash, role) VALUES
    ('Admin Sekolah', '198000000001', 'admin@sekolah.sch.id',
     '$2a$10$CwTycUXWue0Thq9StjUM0uJ8Rz6d0j7oB3v9Vf1IOF6EbKoNlHZDe', 'admin'),
    ('Budi Santoso', '198500000002', 'budi@sekolah.sch.id',
     '$2a$10$CwTycUXWue0Thq9StjUM0uJ8Rz6d0j7oB3v9Vf1IOF6EbKoNlHZDe', 'guru'),
    ('Siti Aminah', '199000000003', 'siti@sekolah.sch.id',
     '$2a$10$CwTycUXWue0Thq9StjUM0uJ8Rz6d0j7oB3v9Vf1IOF6EbKoNlHZDe', 'guru'),
    ('Dedi Guru Pengganti', '199500000004', 'dedi.inval@sekolah.sch.id',
     '$2a$10$CwTycUXWue0Thq9StjUM0uJ8Rz6d0j7oB3v9Vf1IOF6EbKoNlHZDe', 'guru_pengganti');

INSERT INTO rooms (name, qr_string) VALUES
    ('Ruang 10 IPA 1', 'ROOM-A1B2C3D4'),
    ('Ruang 10 IPA 2', 'ROOM-E5F6G7H8'),
    ('Lab Komputer', 'ROOM-I9J0K1L2');

-- Jadwal Budi Santoso (teacher_id=2) tiap Senin (day_of_week=1) jam 09:00-10:30 di Ruang 10 IPA 1
INSERT INTO schedules (teacher_id, room_id, day_of_week, start_time, end_time, target_jp, subject) VALUES
    (2, 1, 1, '09:00:00', '10:30:00', 2, 'Matematika'),
    (3, 2, 1, '08:00:00', '09:30:00', 2, 'Bahasa Indonesia');

-- Contoh cuti Budi Santoso (teacher_id=2) yang sudah disetujui hari ini, supaya
-- Dedi Guru Pengganti (teacher_id=4) bisa dites melakukan presensi sebagai
-- pengganti lewat POST /api/attendance/scan-in dengan body substitute_for_id=2.
INSERT INTO leaves (teacher_id, start_date, end_date, leave_type, reason, status, approved_by, approved_at) VALUES
    (2, CURDATE(), CURDATE(), 'sakit', 'Demam, istirahat dokter', 'approved', 1, NOW());
