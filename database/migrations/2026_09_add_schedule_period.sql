-- Migrasi: Jadwal Mengajar sekarang terikat bulan/periode ("berlaku untuk
-- bulan tertentu"), bukan berlaku selamanya seperti sebelumnya. Ini
-- memungkinkan admin menyusun jadwal berbeda tiap bulan/semester, sambil
-- riwayat jadwal bulan-bulan sebelumnya tetap tersimpan & bisa dilihat lagi
-- (filter bulan di halaman Jadwal Mengajar).
--
-- Jalankan ini HANYA jika database Anda sudah pernah dibuat sebelum
-- perubahan ini ada (schema.sql yang baru sudah mengikutsertakannya untuk
-- instalasi baru, jadi skip file ini untuk instalasi baru):
--
--   mysql -u root -p absensi_guru < database/migrations/2026_09_add_schedule_period.sql

USE absensi_guru;

ALTER TABLE schedules
    ADD COLUMN period_month TINYINT UNSIGNED NULL AFTER day_of_week,
    ADD COLUMN period_year  SMALLINT UNSIGNED NULL AFTER period_month;

-- Jadwal yang sudah ada (dibuat sebelum fitur ini) dianggap berlaku untuk
-- BULAN INI, supaya tidak tiba-tiba hilang dari tampilan & tetap dipakai
-- sebagai acuan target JP absensi hari ini. Kalau perlu jadwal yang sama
-- juga berlaku di bulan depan, admin tinggal duplikasi lewat dashboard.
UPDATE schedules
SET period_month = MONTH(CURDATE()), period_year = YEAR(CURDATE())
WHERE period_month IS NULL OR period_year IS NULL;

ALTER TABLE schedules
    MODIFY COLUMN period_month TINYINT UNSIGNED NOT NULL,
    MODIFY COLUMN period_year  SMALLINT UNSIGNED NOT NULL,
    ADD INDEX idx_schedules_period (period_year, period_month);
