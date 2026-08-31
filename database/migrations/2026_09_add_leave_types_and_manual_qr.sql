-- Migrasi: (1) QR Code ruangan sekarang HANYA berganti lewat tombol
-- "Refresh Sekarang" yang ditekan admin secara manual -- tidak ada lagi
-- rotasi otomatis di background/saat halaman dibuka, supaya QR yang sudah
-- dicetak/ditempel di kelas tetap valid. (2) Jenis cuti/izin sekarang jadi
-- master data yang dikelola Admin lewat dashboard, bukan hardcode di
-- frontend lagi.
--
-- Jalankan ini HANYA jika database Anda sudah pernah dibuat sebelum
-- perubahan ini ada (schema.sql yang baru sudah mengikutsertakannya untuk
-- instalasi baru, jadi skip file ini untuk instalasi baru):
--
--   mysql -u root -p absensi_guru < database/migrations/2026_09_add_leave_types_and_manual_qr.sql

USE absensi_guru;

-- --- (1) QR manual-only ---------------------------------------------
-- Kolom qr_expires_at tidak lagi dipakai (QR tidak pernah kedaluwarsa
-- sendiri), tapi dibiarkan ada di database (tidak di-drop) supaya migrasi
-- ini aman dijalankan tanpa risiko kehilangan data pada instalasi yang
-- sedang berjalan. Kode backend sudah berhenti membaca/menulis kolom ini.
-- Setting qr_rotation_seconds juga sudah tidak dipakai lagi oleh kode, boleh
-- dibiarkan atau dihapus manual jika diinginkan:
--   DELETE FROM settings WHERE config_key = 'qr_rotation_seconds';

-- --- (2) Master data jenis cuti/izin ----------------------------------
CREATE TABLE IF NOT EXISTS leave_types (
    id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    code         VARCHAR(50)   NOT NULL UNIQUE,
    label        VARCHAR(100)  NOT NULL,
    is_active    TINYINT(1)    NOT NULL DEFAULT 1,
    created_at   DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP
                                ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB;

-- Seed dengan jenis yang dulunya hardcode di frontend, supaya data leaves
-- lama ('sakit', 'dinas_luar') tetap konsisten dengan master data baru ini.
INSERT INTO leave_types (code, label) VALUES
    ('sakit', 'Sakit'),
    ('dinas_luar', 'Izin Dinas Luar')
ON DUPLICATE KEY UPDATE code = code;

-- Kalau ada leave_type lain yang sudah pernah dipakai di tabel leaves tapi
-- belum ada di master data (misalnya diinput manual lewat Postman), ikutkan
-- juga supaya tidak "hilang" dari master data.
INSERT INTO leave_types (code, label)
SELECT DISTINCT l.leave_type, l.leave_type
FROM leaves l
LEFT JOIN leave_types lt ON lt.code = l.leave_type
WHERE lt.id IS NULL;
