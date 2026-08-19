# Backend - Sistem Absensi Jam Mengajar Guru (QR Code)

Backend API untuk sistem absensi guru, dibangun dengan **Golang + MySQL + JWT**,
sesuai SRS. Bagian: **Wahyu (Backend Developer)**.

## 1. Struktur Project

```
absensi-backend/
├── main.go                  # entry point
├── config/database.go       # koneksi MySQL
├── models/models.go         # struct data (User, Room, Schedule, Leave, Attendance)
├── middleware/auth.go       # JWT auth middleware + role guard
├── utils/jwt.go             # generate & parse JWT
├── utils/response.go        # helper response JSON seragam
├── handlers/
│   ├── auth_handler.go        # POST /api/auth/login
│   ├── teacher_handler.go     # CRUD guru
│   ├── room_handler.go        # CRUD ruangan + generator QR string
│   ├── schedule_handler.go    # CRUD jadwal
│   ├── attendance_handler.go  # scan-in / scan-out (LOGIKA UTAMA)
│   ├── leave_handler.go       # pengajuan & approval cuti
│   └── report_handler.go      # rekap bulanan & history log
├── routes/routes.go          # semua routing + middleware chain
├── database/schema.sql       # DDL MySQL
├── database/seed.sql         # data contoh untuk testing
├── postman_collection.json   # dokumentasi API siap import Postman
└── .env.example
```

## 2. Setup — Database di TiDB Cloud (tidak perlu XAMPP lagi)

### 2.1 Buat cluster & ambil kredensial

1. Buka [tidbcloud.com](https://tidbcloud.com/), login/daftar, lalu buat cluster
   **Serverless** (gratis, cukup untuk project ini).
2. Di dashboard cluster, klik **Connect**.
3. Di panel yang muncul: pilih **Connect With → General**, lalu (kalau ada
   pilihan driver) pilih **Go**.
4. Klik **Generate Password** kalau belum pernah — TiDB Cloud hanya
   menampilkan password sekali, jadi simpan langsung.
5. Catat 4 nilai berikut dari panel itu:
   - **Host** (`gateway01.xx-xxxxx-x.prod.aws.tidbcloud.com`)
   - **Port** (`4000`)
   - **User** (formatnya `<random>.root`, BUKAN cuma `root`)
   - **Password** (yang baru di-generate)

### 2.2 Isi `.env`

```bash
cp .env.example .env
```

Edit `.env`, isi blok TiDB Cloud dengan 4 nilai di atas:

```
DB_USER=xxxxxxxx.root
DB_PASSWORD=<password dari TiDB Cloud>
DB_HOST=gateway01.xx-xxxxx-x.prod.aws.tidbcloud.com
DB_PORT=4000
DB_NAME=absensi_guru
DB_TLS_MODE=true
```

`DB_TLS_MODE=true` wajib untuk TiDB Cloud — koneksi tanpa TLS akan ditolak.
(Kalau suatu saat balik pakai MySQL lokal/XAMPP, tinggal isi `DB_HOST=127.0.0.1`,
`DB_PORT=3306`, `DB_TLS_MODE=false`.)

### 2.3 Buat schema & seed data di TiDB

Cara termudah: buka **SQL Editor bawaan di dashboard TiDB Cloud** (di menu
cluster kamu), lalu copy-paste isi `database/schema.sql`, jalankan, lalu
copy-paste isi `database/seed.sql`, jalankan. Kalau ada, jalankan juga isi
`database/migrations/2026_08_add_guru_pengganti_role.sql`.

Atau lewat terminal, kalau sudah punya MySQL client terinstal (client saja,
bukan server XAMPP):

```bash
mysql --comment -u '<DB_USER dari .env>' -h <DB_HOST> -P 4000 \
  -D test --ssl-mode=VERIFY_IDENTITY -p < database/schema.sql

mysql --comment -u '<DB_USER dari .env>' -h <DB_HOST> -P 4000 \
  -D absensi_guru --ssl-mode=VERIFY_IDENTITY -p < database/seed.sql

mysql --comment -u '<DB_USER dari .env>' -h <DB_HOST> -P 4000 \
  -D absensi_guru --ssl-mode=VERIFY_IDENTITY -p < database/migrations/2026_08_add_guru_pengganti_role.sql
```

(Masukkan password waktu diminta. `schema.sql` sendiri sudah berisi
`CREATE DATABASE IF NOT EXISTS absensi_guru`, jadi run pertama boleh target
database bawaan `test` — setelah itu database `absensi_guru` sudah ada dan
dipakai untuk perintah berikutnya.)

### 2.4 Install dependency & jalankan server

```bash
go mod tidy
export $(cat .env | xargs)   # atau pakai lib seperti godotenv
go run main.go
```

Kalau berhasil, akan muncul log `berhasil konek ke database MySQL: absensi_guru`
— tapi sebenarnya sudah nyambung ke cluster TiDB Cloud kamu, bukan MySQL lokal.

Server berjalan di `http://localhost:8080`. Semua endpoint di bawah prefix `/api`.

Akun contoh dari `seed.sql` (password semua: `password123`):
| NIP | Role |
|---|---|
| 198000000001 | admin |
| 198500000002 | guru (Budi Santoso) |
| 199000000003 | guru (Siti Aminah) |
| 199500000004 | guru_pengganti (Dedi Guru Pengganti) |

### Role `guru_pengganti`

Ditambahkan sebagai role ke-3 (selain `admin` dan `guru`) sesuai aktor di SRS:
> "Guru Pengganti (Inval): Melakukan presensi untuk jadwal guru utama yang
> sedang berhalangan/cuti."

- Dibuat/dikelola lewat endpoint yang sama dengan guru biasa
  (`POST/PUT /api/admin/teachers` dengan `"role": "guru_pengganti"`).
- Route `/api/attendance/*` dan `/api/leaves` sekarang eksplisit hanya
  menerima role `guru` dan `guru_pengganti` (lewat `middleware.RequireAnyRole`)
  — akun `admin` murni tidak lagi bisa memanggil endpoint presensi/cuti.
  Sebelumnya endpoint ini bisa diakses siapa saja yang login, tanpa cek role.
- **Validasi baru**: `POST /api/attendance/scan-in` dengan `substitute_for_id`
  sekarang memverifikasi guru utama (`substitute_for_id`) memang punya
  pengajuan cuti berstatus `approved` yang mencakup tanggal hari ini. Sebelumnya
  siapa pun yang login bisa klaim jadi pengganti guru mana pun tanpa validasi
  ini — celah yang sekarang ditutup. Kalau belum ada cuti approved, responsenya
  403 dengan pesan `"Guru utama tidak dalam status cuti yang disetujui hari
  ini, tidak bisa presensi sebagai pengganti"`.
- Baik akun `guru` maupun `guru_pengganti` sama-sama boleh memicu mode
  pengganti (kirim `substitute_for_id`) — desain ini sengaja dibuat fleksibel:
  guru biasa kadang menggantikan kolega secara dadakan, sementara akun
  `guru_pengganti` cocok untuk staf yang memang tugasnya jadi cadangan/inval
  dan tidak punya jadwal tetap sendiri.
- Kalau database sudah pernah dibuat sebelum role ini ada, jalankan migrasi:
  `mysql -u root -p absensi_guru < database/migrations/2026_08_add_guru_pengganti_role.sql`

> Catatan: hash password di `seed.sql` adalah contoh/placeholder. Untuk akun
> production sebaiknya buat lewat endpoint `POST /api/admin/teachers` agar
> di-hash dengan bcrypt yang benar-benar dihasilkan server.

## 3. Dokumentasi API (Postman)

Import `postman_collection.json` ke Postman. Sudah termasuk:
- Semua endpoint sesuai draft SRS + tambahan (history, my-leaves, approve/reject cuti).
- Auto-save JWT token ke variable `{{token}}` setelah request **Login** berhasil,
  jadi Akbar & Daniel tinggal jalankan Login sekali lalu semua request lain
  otomatis terauth.
- Contoh body untuk tiap request (termasuk mode guru pengganti).

## 4. Logika Perhitungan Jam Mengajar (Inti dari SRS §4)

Implementasi lengkap ada di `handlers/attendance_handler.go`.

**Scan Masuk (`ScanIn`)**
- Waktu **selalu** diambil dari `time.Now()` di server — endpoint tidak pernah
  menerima timestamp dari client, sehingga fake device clock di Flutter otomatis
  tidak berpengaruh (validasi GPS palsu/timestamp tetap harus dicek di sisi Flutter
  sebelum request dikirim, sesuai requirement "menolak Fake GPS").
- Mencari jadwal guru (atau guru yang digantikan, jika `substitute_for_id` diisi)
  pada hari & ruangan yang sama, memilih jadwal yang sedang berlangsung atau
  jadwal terdekat ke depan yang sudah masuk masa toleransi (`early_scan_tolerance_minutes`
  dari tabel `settings`).
- `clock_in` dicatat persis sesuai waktu aktual scan; presensi tetap **diikat**
  ke `schedule_id` jadwal yang dipilih (bukan waktu scan).
- Jika belum masuk masa toleransi → error `"Jadwal Anda belum dimulai"`.

**Scan Keluar (`ScanOut`)**
- `clock_out` dicatat persis sesuai waktu aktual scan.
- **Waktu efektif mulai**: jam jadwal mulai (menit menunggu sebelum jadwal tidak
  dihitung) — kecuali guru clock-in setelah jadwal mulai, maka efektif mulai
  dari `clock_in` aktual (asumsi tambahan agar guru yang telat tidak "untung").
- **Waktu efektif selesai**: nilai yang lebih cepat antara jadwal selesai resmi
  vs waktu aktual scan out.
- **Pembulatan JP**: `floor(durasi_menit / jp_duration_minutes)`, sesuai contoh
  di SRS (60 menit / 45 menit = 1 JP, bukan 1.33).
- Status otomatis `"tuntas"` jika `actual_jp >= target_jp`, selain itu
  `"tidak_tuntas"`.
- Response memberi flag `early_clock_out: true/false` supaya Flutter bisa
  menampilkan pop-up peringatan sebelum request dikirim (idealnya dicek di
  Flutter side sebelum panggil API, popup di SRS adalah UX preventif).

**Konfigurasi** (`settings` table, bisa diubah admin):
- `jp_duration_minutes` (default 45)
- `early_scan_tolerance_minutes` (default 60)

## 5. Autentikasi & Otorisasi

- Login (`POST /api/auth/login`) pakai NIP + password (bcrypt), mengembalikan
  JWT berlaku 12 jam.
- Semua endpoint lain (kecuali login) wajib header `Authorization: Bearer <token>`.
- Endpoint `/api/admin/*` tambahan mewajibkan `role = admin` (lihat
  `middleware.RequireRole`).
- **WAJIB** ganti `JWT_SECRET` di `.env` sebelum deploy ke production.

## 6. Endpoint yang Tersedia

| Method | Endpoint | Akses | Keterangan |
|---|---|---|---|
| POST | `/api/auth/login` | Publik | Login, dapat JWT |
| GET/POST | `/api/admin/teachers` | Admin | CRUD guru |
| PUT/DELETE | `/api/admin/teachers/{id}` | Admin | Update / nonaktifkan guru |
| GET/POST | `/api/admin/rooms` | Admin | CRUD ruangan + generate QR |
| PUT/DELETE | `/api/admin/rooms/{id}` | Admin | Update / nonaktifkan ruangan |
| GET/POST | `/api/admin/schedules` | Admin | CRUD jadwal |
| PUT/DELETE | `/api/admin/schedules/{id}` | Admin | Update / nonaktifkan jadwal |
| GET | `/api/admin/leaves` | Admin | Semua pengajuan cuti |
| PUT | `/api/admin/leaves/{id}/approve` | Admin | Setujui cuti |
| PUT | `/api/admin/leaves/{id}/reject` | Admin | Tolak cuti |
| GET | `/api/admin/reports/monthly?month=&year=` | Admin | Rekap bulanan per guru |
| GET | `/api/admin/reports/history?teacher_id=&start=&end=` | Admin | History log detail scan |
| POST | `/api/attendance/scan-in` | Guru | Scan masuk (+ mode inval via `substitute_for_id`) |
| POST | `/api/attendance/scan-out` | Guru | Scan keluar |
| GET | `/api/attendance/history` | Guru | Riwayat presensi sendiri |
| POST | `/api/leaves` | Guru | Ajukan cuti/izin |
| GET | `/api/leaves` | Guru | Riwayat cuti sendiri |

## 7. Catatan untuk Akbar (Flutter) & Daniel (Svelte)

- Semua response berformat seragam:
  ```json
  { "success": true, "message": "...", "data": { ... } }
  ```
- Untuk **Generator QR Code Ruangan**: panggil `POST /api/admin/rooms`, field
  `qr_string` di response itulah yang di-encode jadi QR image di sisi Svelte
  untuk dicetak (bisa pakai library JS seperti `qrcode` untuk generate image-nya;
  backend hanya menyediakan string uniknya).
- Untuk **export Excel/PDF** di rekap bulanan: endpoint `/admin/reports/monthly`
  sudah mengembalikan data mentah dalam JSON; generate file Excel/PDF-nya
  disarankan di sisi Svelte (mis. SheetJS) supaya backend tetap ringan — kalau
  memang perlu digenerate di server, kabari saya untuk ditambahkan endpoint
  export terpisah.
- Field `bound_to` di response scan-in menunjukkan jadwal yang diikat (jam mulai
  jadwal), bukan jam scan aktual — bisa dipakai Flutter untuk menampilkan
  konfirmasi ke guru.

## 8. Yang Belum Termasuk (perlu didiskusikan lebih lanjut)

- Upload file lampiran foto untuk pengajuan cuti (saat ini hanya menerima URL
  string `attachment_url` — perlu diputuskan mekanisme upload-nya, mis. S3/local
  storage terpisah).
- Rate limiting / refresh token (saat ini token cukup 12 jam sekali login ulang).
- Validasi GPS & fake-GPS detection dilakukan di sisi **Flutter** sebelum kirim
  request scan (backend hanya menjamin waktu server, sesuai SRS §3B).

## 9. Deploy ke Railway (hosting publik untuk backend)

Server yang jalan di laptop kamu (`localhost:8080`) hanya bisa diakses dari
laptop itu sendiri. Supaya frontend yang di-hosting di Vercel bisa memanggil
API ini, backend perlu jalan di alamat publik. Railway pilihan yang paling
simpel untuk Go app kecil seperti ini.

### 9.1 Push project ini ke GitHub

Railway deploy langsung dari repo GitHub. Kalau belum ada repo:

```bash
git init
git add .
git commit -m "initial commit"
# buat repo baru di github.com, lalu:
git remote add origin https://github.com/<username>/<repo-name>.git
git push -u origin main
```

**Penting:** jangan commit file `.env` (isi kredensial database asli). File
`.gitignore` sudah ada — cek dulu `.env` masuk di situ.

### 9.2 Buat project di Railway

1. Buka [railway.app](https://railway.app/), login (bisa pakai akun GitHub).
2. **New Project → Deploy from GitHub repo** → pilih repo backend ini.
3. Railway otomatis mendeteksi ini project Go (lewat `go.mod`) dan akan
   build & jalankan otomatis pakai Nixpacks — tidak perlu Dockerfile.

### 9.3 Isi environment variables di Railway

Di dashboard project → tab **Variables**, tambahkan persis seperti isi
`.env` kamu (kredensial TiDB Cloud yang sama):

```
DB_USER=jbiSFgA6pPfVLrt.root
DB_PASSWORD=<password TiDB Cloud kamu>
DB_HOST=gateway01.ap-southeast-1.prod.aws.tidbcloud.com
DB_PORT=4000
DB_NAME=absensi_guru
DB_TLS_MODE=true
JWT_SECRET=<ganti dengan string acak yang panjang>
CORS_ALLOWED_ORIGINS=https://<domain-vercel-kamu>.vercel.app,http://localhost:5173
```

Railway otomatis menyediakan env var `PORT` sendiri — jangan di-set manual,
app ini sudah otomatis pakai `PORT` dari Railway kalau ada.

`CORS_ALLOWED_ORIGINS` boleh diisi beberapa origin dipisah koma — isi dengan
URL Vercel kamu setelah frontend-nya di-deploy (bisa diedit belakangan).

### 9.4 Deploy & cek

Railway akan build otomatis setiap kali kamu push ke GitHub. Setelah build
sukses, buka tab **Settings → Networking → Generate Domain** untuk dapat URL
publik, misalnya `https://absensi-backend-production.up.railway.app`.

Cek dengan buka `https://<url-railway-kamu>/api/auth/login` di browser —
kalau muncul respons JSON (walau error "method not allowed" karena harusnya
POST), berarti server sudah hidup dan reachable dari internet.

Pakai URL itu (+ `/api`) sebagai `VITE_API_BASE_URL` di frontend Vercel kamu.
