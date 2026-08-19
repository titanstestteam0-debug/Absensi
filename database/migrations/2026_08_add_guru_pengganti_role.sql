-- Migrasi: menambahkan role 'guru_pengganti' ke tabel users.
-- Jalankan ini HANYA jika database Anda sudah pernah dibuat sebelum role
-- ini ditambahkan (schema.sql yang baru sudah mengikutsertakan role ini
-- untuk instalasi baru, jadi skip file ini untuk instalasi baru).
--
--   mysql -u root -p absensi_guru < database/migrations/2026_08_add_guru_pengganti_role.sql

USE absensi_guru;

ALTER TABLE users
    MODIFY COLUMN role ENUM('admin', 'guru', 'guru_pengganti') NOT NULL DEFAULT 'guru';
