-- Migrasi: tambah kolom `tagline` di school_settings -- subjudul kecil di
-- bawah nama sekolah pada header aplikasi (mis. "Sistem Presensi Mengajar
-- berbasis QR Code"). NULL berarti tampil teks bawaan aplikasi.
--
-- Jalankan ini HANYA jika database Anda sudah pernah dibuat sebelum
-- perubahan ini ada (schema.sql yang baru sudah mengikutsertakannya untuk
-- instalasi baru, jadi skip file ini untuk instalasi baru):
--
--   mysql -u root -p absensi_guru < database/migrations/2026_09_add_school_tagline.sql

USE absensi_guru;

ALTER TABLE school_settings
    ADD COLUMN IF NOT EXISTS tagline VARCHAR(150) NULL AFTER logo_data_url;
