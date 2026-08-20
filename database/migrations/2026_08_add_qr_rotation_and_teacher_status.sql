-- Migrasi: menambahkan dukungan QR Code yang berotasi otomatis (anti foto/screenshot
-- dari rumah), dan setting terkait. Kolom is_active di tabel users (status akun
-- guru: Aktif/Nonaktif) sudah ada sejak schema awal, jadi tidak perlu migrasi
-- tambahan untuk itu -- hanya perlu endpoint baru (lihat handlers/teacher_handler.go).
--
-- Jalankan ini HANYA jika database Anda sudah pernah dibuat sebelum kolom-kolom
-- ini ditambahkan (schema.sql yang baru sudah mengikutsertakannya untuk instalasi
-- baru, jadi skip file ini untuk instalasi baru):
--
--   mysql -u root -p absensi_guru < database/migrations/2026_08_add_qr_rotation_and_teacher_status.sql

USE absensi_guru;

ALTER TABLE rooms
    ADD COLUMN qr_expires_at      DATETIME NULL AFTER qr_string,
    ADD COLUMN qr_last_rotated_at DATETIME NULL AFTER qr_expires_at;

INSERT INTO settings (config_key, config_value, description) VALUES
    ('qr_rotation_seconds', '20', 'Interval otomatis QR Code ruangan berganti (detik), agar QR yang difoto/discreenshot cepat kedaluwarsa dan tidak bisa dipakai scan dari rumah')
ON DUPLICATE KEY UPDATE config_key = config_key;

-- Set semua ruangan yang sudah ada supaya QR-nya langsung dianggap kedaluwarsa,
-- sehingga saat pertama kali dibuka lagi di halaman admin, backend otomatis
-- merotasi ke QR baru (lazy rotation di GetRoomQR).
UPDATE rooms SET qr_expires_at = NOW() WHERE qr_expires_at IS NULL;
