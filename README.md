# PostgreSQL Sharding with Master-Replica Setup in Go

โครงการนี้เป็นตัวอย่างการทำ Database Sharding ด้วย PostgreSQL โดยใช้ Hash Partitioning และการตั้งค่า Master-Replica configuration พร้อมแอพพลิเคชัน Go ที่ใช้ GORM และระบบ Monitoring ด้วย Prometheus + Grafana

## 🏗️ สถาปัตยกรรมระบบ

```mermaid
graph TB
    subgraph "Application Layer"
        App[Go Application<br/>GORM + DBResolver<br/>Port: 8080]
    end

    subgraph "Database Layer"
        Master[Master Database<br/>Port: 5432<br/>✅ Read/Write]
        Replica1[Replica 1<br/>Port: 5433<br/>📖 Read Only]
        Replica2[Replica 2<br/>Port: 5434<br/>📖 Read Only]
    end

    subgraph "Monitoring Layer"
        Prometheus[Prometheus<br/>Port: 9090<br/>📊 Metrics Collection]
        Grafana[Grafana<br/>Port: 3000<br/>📈 Dashboard]
        NodeExporter[Node Exporter<br/>Port: 9100<br/>🖥️ System Metrics]
        Cadvisor[cAdvisor<br/>Port: 8080<br/>🐳 Container Metrics]
    end

    subgraph "PostgreSQL Exporters"
        PGExpMaster[PostgreSQL Exporter<br/>Master - Port: 9187]
        PGExpReplica1[PostgreSQL Exporter<br/>Replica 1 - Port: 9188]
        PGExpReplica2[PostgreSQL Exporter<br/>Replica 2 - Port: 9189]
    end

    subgraph "Hash Partitions (ทุก Database)"
        P0[tbl_users_p0<br/>hash % 4 = 0]
        P1[tbl_users_p1<br/>hash % 4 = 1]
        P2[tbl_users_p2<br/>hash % 4 = 2]
        P3[tbl_users_p3<br/>hash % 4 = 3]
    end

    %% Application to Database connections
    App -->|Write Operations| Master
    App -->|Read Operations| Replica1
    App -->|Read Operations| Replica2

    %% Replication connections
    Master -.->|WAL Replication| Replica1
    Master -.->|WAL Replication| Replica2

    %% Monitoring connections
    Prometheus -->|Scrape Metrics| PGExpMaster
    Prometheus -->|Scrape Metrics| PGExpReplica1
    Prometheus -->|Scrape Metrics| PGExpReplica2
    Prometheus -->|Scrape Metrics| NodeExporter
    Prometheus -->|Scrape Metrics| Cadvisor

    %% Database to Exporter connections
    PGExpMaster -->|Query Metrics| Master
    PGExpReplica1 -->|Query Metrics| Replica1
    PGExpReplica2 -->|Query Metrics| Replica2

    %% Grafana to Prometheus
    Grafana -->|Query Data| Prometheus

    %% Partition relationships
    Master -.-> P0
    Master -.-> P1
    Master -.-> P2
    Master -.-> P3
    Replica1 -.-> P0
    Replica1 -.-> P1
    Replica1 -.-> P2
    Replica1 -.-> P3
    Replica2 -.-> P0
    Replica2 -.-> P1
    Replica2 -.-> P2
    Replica2 -.-> P3

    %% Styling
    classDef appClass fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef dbClass fill:#f3e5f5,stroke:#4a148c,stroke-width:2px
    classDef monitorClass fill:#e8f5e8,stroke:#1b5e20,stroke-width:2px
    classDef exporterClass fill:#fff3e0,stroke:#e65100,stroke-width:2px
    classDef partitionClass fill:#fce4ec,stroke:#880e4f,stroke-width:2px

    class App appClass
    class Master,Replica1,Replica2 dbClass
    class Prometheus,Grafana,NodeExporter,Cadvisor monitorClass
    class PGExpMaster,PGExpReplica1,PGExpReplica2 exporterClass
    class P0,P1,P2,P3 partitionClass
```

### การทำงานของระบบ

| Component                 | Port | หน้าที่                | การเชื่อมต่อ                             |
| ------------------------- | ---- | ---------------------- | ---------------------------------------- |
| **Go Application**        | 8080 | Business Logic         | GORM + DBResolver                        |
| **Master DB**             | 5432 | Write + Read           | Go App → Master (INSERT, UPDATE, DELETE) |
| **Replica 1**             | 5433 | Read Only              | Go App → Replica 1 (SELECT)              |
| **Replica 2**             | 5434 | Read Only              | Go App → Replica 2 (SELECT)              |
| **Prometheus**            | 9090 | Metrics Collection     | Scrape จาก Exporters                     |
| **Grafana**               | 3000 | Dashboard              | Query จาก Prometheus                     |
| **Node Exporter**         | 9100 | System Metrics         | CPU, Memory, Disk                        |
| **cAdvisor**              | 8080 | Container Metrics      | Docker Stats                             |
| **PG Exporter Master**    | 9187 | DB Metrics (Master)    | PostgreSQL Statistics                    |
| **PG Exporter Replica 1** | 9188 | DB Metrics (Replica 1) | PostgreSQL Statistics                    |
| **PG Exporter Replica 2** | 9189 | DB Metrics (Replica 2) | PostgreSQL Statistics                    |

## 📋 รายละเอียดไฟล์

### 1. `docker-compose.yml` - การจัดการ Container

**Master Database (lines 4-28):**

```yaml
master-db:
  image: postgres:17
  ports: - 5432:5432
  environment:
    POSTGRES_PASSWORD: postgres
    POSTGRES_USER: postgres
    POSTGRES_DB: postgres
```

**การตั้งค่า Volume Mount (lines 12-17):**

- `./init-master.sql` → สร้างตาราง partitions และ replication user
- `./pg_hba.conf` → การตั้งค่าสิทธิ์การเชื่อมต่อ
- `./postgresql.conf` → การตั้งค่า replication
- `./setup-config.sh` → script สำหรับนำ config files ไปใช้

**Health Check (lines 20-28):**

```bash
# ตรวจสอบว่า PostgreSQL พร้อมและมี 4 partitions ถูกสร้างแล้ว
pg_isready -U postgres && psql -U postgres -c "SELECT count(*) FROM pg_tables WHERE tablename LIKE 'tbl_users_p%';" | grep -q '4'
```

**Replica Databases (lines 30-138):**

- **Replica-One**: Port 5433
- **Replica-Two**: Port 5434

**การตั้งค่า Replica (lines 45-72, 98-129):**

```bash
# รอให้ master สร้าง partitions เสร็จ
until PGPASSWORD=postgres psql -h master-db -U postgres -c "SELECT count(*) FROM pg_tables WHERE tablename LIKE 'tbl_users_p%';" | grep -q '4'

# ทำ pg_basebackup จาก master
PGPASSWORD=replicator_password pg_basebackup -h master-db -D /var/lib/postgresql/data -U replicator -v -P

# ตั้งค่า replication สำหรับ PostgreSQL 17
echo "primary_conninfo = 'host=master-db port=5432 user=replicator password=replicator_password'" >> /var/lib/postgresql/data/postgresql.auto.conf
echo "hot_standby = on" >> /var/lib/postgresql/data/postgresql.auto.conf

# สร้าง standby.signal file (แทน standby_mode = on ใน version เก่า)
touch /var/lib/postgresql/data/standby.signal
```

**Prometheus (lines 140-157):**

```yaml
prometheus:
  image: prom/prometheus:latest
  ports: - 9090:9090
  volumes:
    - ./prometheus.yml:/etc/prometheus/prometheus.yml
    - prometheus-data:/prometheus
  command:
    - "--config.file=/etc/prometheus/prometheus.yml"
    - "--storage.tsdb.path=/prometheus"
    - "--web.enable-lifecycle"
    - "--web.enable-admin-api"
```

**Grafana (lines 159-175):**

```yaml
grafana:
  image: grafana/grafana:latest
  ports: - 3000:3000
  environment:
    - GF_SECURITY_ADMIN_PASSWORD=admin
    - GF_INSTALL_PLUGINS=grafana-clock-panel,grafana-simple-json-datasource
  volumes:
    - grafana-data:/var/lib/grafana
    - ./grafana/provisioning:/etc/grafana/provisioning
    - ./grafana/dashboards:/etc/grafana/dashboards
```

**Node Exporter (lines 177-192):**

```yaml
node-exporter:
  image: prom/node-exporter:latest
  ports: - 9100:9100
  volumes:
    - /proc:/host/proc:ro
    - /sys:/host/sys:ro
    - /:/rootfs:ro
  command:
    - "--path.procfs=/host/proc"
    - "--path.sysfs=/host/sys"
    - "--collector.filesystem.mount-points-exclude=^/(sys|proc|dev|host|etc)($$|/)"
```

**cAdvisor (lines 194-210):**

```yaml
cadvisor:
  image: gcr.io/cadvisor/cadvisor:latest
  ports: - 8080:8080
  volumes:
    - /:/rootfs:ro
    - /var/run:/var/run:ro
    - /sys:/sys:ro
    - /var/lib/docker/:/var/lib/docker:ro
    - /dev/disk/:/dev/disk:ro
  privileged: true
  devices: - /dev/kmsg
```

**PostgreSQL Exporters (lines 212-256):**

```yaml
# Master Database Exporter
postgres-exporter-master:
  image: prometheuscommunity/postgres-exporter:latest
  ports: - 9187:9187
  environment:
    DATA_SOURCE_NAME: "postgresql://postgres:postgres@master-db:5432/postgres?sslmode=disable"

# Replica One Exporter
postgres-exporter-replica-one:
  image: prometheuscommunity/postgres-exporter:latest
  ports: - 9188:9187
  environment:
    DATA_SOURCE_NAME: "postgresql://postgres:postgres@replica-one:5432/postgres?sslmode=disable"

# Replica Two Exporter
postgres-exporter-replica-two:
  image: prometheuscommunity/postgres-exporter:latest
  ports: - 9189:9187
  environment:
    DATA_SOURCE_NAME: "postgresql://postgres:postgres@replica-two:5432/postgres?sslmode=disable"
```

### 2. `init-master.sql` - การตั้งค่าฐานข้อมูล Master

**Replication User (lines 1-5):**

```sql
-- สร้าง user สำหรับ replication
CREATE USER replicator REPLICATION LOGIN CONNECTION LIMIT 5 ENCRYPTED PASSWORD 'replicator_password';
GRANT CONNECT ON DATABASE postgres TO replicator;
```

**Monitoring User (lines 7-10):**

```sql
-- สร้าง user สำหรับ postgres_exporter
CREATE USER postgres_exporter PASSWORD 'postgres_exporter_password';
GRANT CONNECT ON DATABASE postgres TO postgres_exporter;
GRANT pg_monitor TO postgres_exporter;
```

**Hash Partitioning Setup (lines 14-30):**

```sql
-- สร้าง main table with hash partitioning
CREATE TABLE tbl_users (
    id UUID NOT NULL,
    first_name VARCHAR(255) NOT NULL,
    PRIMARY KEY (id)
) PARTITION BY HASH (id);

-- สร้าง 4 partitions โดยใช้ modulus 4
CREATE TABLE tbl_users_p0 PARTITION OF tbl_users FOR VALUES WITH (MODULUS 4, REMAINDER 0);
CREATE TABLE tbl_users_p1 PARTITION OF tbl_users FOR VALUES WITH (MODULUS 4, REMAINDER 1);
CREATE TABLE tbl_users_p2 PARTITION OF tbl_users FOR VALUES WITH (MODULUS 4, REMAINDER 2);
CREATE TABLE tbl_users_p3 PARTITION OF tbl_users FOR VALUES WITH (MODULUS 4, REMAINDER 3);
```

### 3. `postgresql.conf` - การตั้งค่า PostgreSQL

**Replication Settings (lines 1-6):**

```conf
wal_level = replica              # เปิดใช้ WAL สำหรับ replication
max_wal_senders = 5             # จำนวน sender processes สูงสุด
max_replication_slots = 5       # จำนวน replication slots สูงสุด
hot_standby = on                # อนุญาตให้ read จาก replica
listen_addresses = '*'          # รับ connection จากทุก IP
```

**Performance Settings (lines 8-10):**

```conf
max_connections = 100           # จำนวน connection สูงสุด
shared_buffers = 128MB         # memory สำหรับ shared buffers
```

**Monitoring & Statistics Settings (lines 12-17):**

```conf
track_activities = on           # เปิดใช้ activity tracking
track_counts = on              # เปิดใช้ statistics tracking
track_io_timing = on           # เปิดใช้ I/O timing
track_functions = all          # เปิดใช้ function tracking
shared_preload_libraries = 'pg_stat_statements'  # โหลด pg_stat_statements
```

**Logging Settings (lines 19-23):**

```conf
log_statement = 'all'          # log ทุก SQL statement
log_destination = 'stderr'     # ส่ง log ไป stderr
logging_collector = on         # เปิด log collector
log_filename = 'postgresql-%Y-%m-%d_%H%M%S.log'  # รูปแบบชื่อไฟล์ log
```

### 4. `pg_hba.conf` - การตั้งค่าสิทธิ์การเชื่อมต่อ

**Local Connections (lines 4-8):**

```conf
local   all             all                                     trust
host    all             all             127.0.0.1/32            md5
host    all             all             ::1/128                 md5
```

**Replication Connections (lines 11-13, 17, 21):**

```conf
local   replication     all                                     trust
host    replication     all             127.0.0.1/32            md5
host    replication     replicator      172.18.0.0/16           md5  # Docker network
host    replication     replicator      0.0.0.0/0               md5  # ทุก IP
```

**Docker Network Access (lines 16, 20):**

```conf
host    all             all             172.18.0.0/16           md5  # Docker internal network
host    all             all             0.0.0.0/0               md5  # External access
```

### 5. `setup-config.sh` - Script การตั้งค่า

**การคัดลอก Config Files (lines 7-8):**

```bash
cp /docker-entrypoint-initdb.d/02-pg_hba.conf /var/lib/postgresql/data/pg_hba.conf
cp /docker-entrypoint-initdb.d/03-postgresql.conf /var/lib/postgresql/data/postgresql.conf
```

**การตั้งค่าสิทธิ์ (lines 11-12):**

```bash
chown postgres:postgres /var/lib/postgresql/data/pg_hba.conf
chown postgres:postgres /var/lib/postgresql/data/postgresql.conf
```

**การโหลด Configuration ใหม่ (lines 17-19):**

```sql
SELECT pg_reload_conf();  -- โหลด config ใหม่โดยไม่ต้อง restart
```

### 6. `prometheus.yml` - การตั้งค่า Prometheus

**Global Settings (lines 1-3):**

```yaml
global:
  scrape_interval: 15s # เก็บ metrics ทุก 15 วินาที
  evaluation_interval: 15s # ประเมิน rules ทุก 15 วินาที
```

**Prometheus Self-Monitoring (lines 10-13):**

```yaml
- job_name: "prometheus"
  static_configs:
    - targets: ["localhost:9090"]
```

**Node Exporter (lines 15-19):**

```yaml
- job_name: "node-exporter"
  static_configs:
    - targets: ["node-exporter:9100"]
  scrape_interval: 10s
```

**cAdvisor (lines 21-25):**

```yaml
- job_name: "cadvisor"
  static_configs:
    - targets: ["cadvisor:8080"]
  scrape_interval: 10s
```

**PostgreSQL Exporters (lines 27-58):**

```yaml
# Master Database
- job_name: "postgres-master"
  static_configs:
    - targets: ["postgres-exporter-master:9187"]
  scrape_interval: 10s
  relabel_configs:
    - source_labels: [__address__]
      target_label: instance
      replacement: "master-db"

# Replica One
- job_name: "postgres-replica-one"
  static_configs:
    - targets: ["postgres-exporter-replica-one:9187"]
  scrape_interval: 10s
  relabel_configs:
    - source_labels: [__address__]
      target_label: instance
      replacement: "replica-one"

# Replica Two
- job_name: "postgres-replica-two"
  static_configs:
    - targets: ["postgres-exporter-replica-two:9187"]
  scrape_interval: 10s
  relabel_configs:
    - source_labels: [__address__]
      target_label: instance
      replacement: "replica-two"
```

### 7. `postgres-exporter-queries.yaml` - Custom Queries

**Database Size Query (lines 2-11):**

```yaml
pg_database_size:
  query: "SELECT pg_database.datname, pg_database_size(pg_database.datname) as size FROM pg_database"
  master: true
  metrics:
    - datname:
        usage: "LABEL"
        description: "Database name"
    - size:
        usage: "GAUGE"
        description: "Database size in bytes"
```

**Connections Query (lines 13-19):**

```yaml
pg_connections:
  query: "SELECT count(*) as connections FROM pg_stat_activity"
  master: true
  metrics:
    - connections:
        usage: "GAUGE"
        description: "Total number of connections"
```

**Activity Query (lines 21-30):**

```yaml
pg_activity:
  query: "SELECT state, count(*) as count FROM pg_stat_activity WHERE state IS NOT NULL GROUP BY state"
  master: true
  metrics:
    - state:
        usage: "LABEL"
        description: "Connection state"
    - count:
        usage: "GAUGE"
        description: "Number of connections in this state"
```

**Partition Monitoring (lines 32-42):**

```yaml
pg_partitions:
  query: "SELECT tablename, schemaname FROM pg_tables WHERE tablename LIKE 'tbl_users_p%'"
  master: true
  metrics:
    - tablename:
        usage: "LABEL"
        description: "Partition table name"
    - schemaname:
        usage: "LABEL"
        description: "Schema name"
```

### 8. Grafana Configuration

**Dashboard Provisioning (`grafana/provisioning/dashboards/default.yml`):**

```yaml
apiVersion: 1
providers:
  - name: "default"
    orgId: 1
    folder: ""
    type: file
    disableDeletion: false
    updateIntervalSeconds: 10
    allowUiUpdates: true
    options:
      path: /etc/grafana/dashboards
```

**Datasource Provisioning (`grafana/provisioning/datasources/prometheus.yml`):**

```yaml
apiVersion: 1
datasources:
  - name: Prometheus
    type: prometheus
    access: proxy
    url: http://prometheus:9090
    isDefault: true
    editable: true
```

**Dashboard JSON (`grafana/dashboards/postgres-dashboard.json`):**

```json
{
  "dashboard": {
    "title": "PostgreSQL Monitoring Dashboard",
    "panels": [
      {
        "title": "Database Connections",
        "type": "stat",
        "targets": [
          {
            "expr": "pg_stat_database_numbackends",
            "legendFormat": "{{datname}}"
          }
        ]
      },
      {
        "title": "PostgreSQL Up Status",
        "type": "stat",
        "targets": [
          {
            "expr": "pg_up",
            "legendFormat": "{{instance}}"
          }
        ]
      }
    ]
  }
}
```

### 9. `go.mod` & `go.sum` - Dependencies

**หลัก Dependencies (lines 5-11 ใน go.mod):**

```go
github.com/caarlos0/env/v11 v11.3.1     // Environment variable parsing
github.com/google/uuid v1.6.0          // UUID generation
gorm.io/driver/postgres v1.6.0         // PostgreSQL driver สำหรับ GORM
gorm.io/gorm v1.30.2                   // ORM framework
gorm.io/plugin/dbresolver v1.6.2       // Master-Replica resolver
```

### 10. `main.go` - แอพพลิเคชัน Go

**User Model (lines 14-29):**

```go
type User struct {
    ID        uuid.UUID `gorm:"column:id;type:uuid;primary_key"`
    FirstName string    `gorm:"column:first_name;type:varchar(255);not null"`
}

func (User) TableName() string {
    return "tbl_users"  // ชี้ไปที่ partitioned table
}

func (u *User) BeforeCreate(tx *gorm.DB) (err error) {
    u.ID, err = uuid.NewV7()  // สร้าง UUID v7 (time-ordered)
    return
}
```

**Environment Configuration (lines 31-53):**

```go
var ENV struct {
    MasterDB struct {
        DBName     string `env:"NAME" envDefault:"postgres"`
        DBHost     string `env:"HOST" envDefault:"localhost"`
        DBPort     string `env:"PORT" envDefault:"5432"`
        DBUser     string `env:"USER" envDefault:"postgres"`
        DBPassword string `env:"PASSWORD" envDefault:"postgres"`
    } `envPrefix:"MASTER_DB_"`

    ReplicaOne struct {
        // Port 5433
    } `envPrefix:"REPLICA_ONE_"`

    ReplicaTwo struct {
        // Port 5434
    } `envPrefix:"REPLICA_TWO_"`
}
```

**Database Connection Setup (lines 84-100):**

```go
func connectDB() *gorm.DB {
    // เชื่อมต่อ master database
    db, err := gorm.Open(postgres.Open(GetDSN(...)), &gorm.Config{})

    // ตั้งค่า DB Resolver สำหรับ master-replica
    db.Use(dbresolver.Register(dbresolver.Config{
        Sources: []gorm.Dialector{postgres.Open(masterDSN)},  // Master สำหรับ write
        Replicas: []gorm.Dialector{
            postgres.Open(replica1DSN),  // Replica 1 สำหรับ read
            postgres.Open(replica2DSN),  // Replica 2 สำหรับ read
        },
        Policy: dbresolver.RandomPolicy{},  // สุ่มเลือก replica สำหรับ read
    }))

    return db
}
```

**Data Insertion (lines 102-114):**

```go
func insertData(db *gorm.DB) {
    users := []User{
        {FirstName: "John"},    // จะถูก hash ไปที่ partition ใดซึ่ง partition หนึ่ง
        {FirstName: "Jane"},    // UUID จะถูก hash และกระจายไปตาม modulus 4
        {FirstName: "Bob"},
        {FirstName: "Alice"},
        {FirstName: "Charlie"},
    }

    db.Create(&users)  // GORM จะเขียนไปที่ master database
}
```

**Partition Query (lines 116-142):**

```go
func queryPartitions(db *gorm.DB) {
    partitions := []string{"tbl_users_p0", "tbl_users_p1", "tbl_users_p2", "tbl_users_p3"}

    for _, partition := range partitions {
        var count int64
        db.Table(partition).Count(&count)  // นับจำนวนในแต่ละ partition

        var users []User
        db.Table(partition).Find(&users)   // อ่านข้อมูลจาก replica (random)
        // แสดงผลการกระจายข้อมูลในแต่ละ partition
    }

    user := User{}
	if err := db.First(&user).Error; err != nil {
		log.Fatalf("error getting user: %v", err)
	}

	uuidTime := user.ID.Time()
	sec, nsec := uuidTime.UnixTime()
	createdTime := time.Unix(sec, nsec)
	fmt.Printf("User: %s (ID: %s, CreatedDate: %s)\n", user.FirstName, user.ID, createdTime.Format("2006-01-02 15:04:05")) // ดูวันที่สร้างข้อมูล
}
```

## 🚀 วิธีการรัน

### 1. เริ่มต้นระบบ

```bash
# สร้างและเริ่ม containers
docker-compose up -d

# ตรวจสอบสถานะ
docker-compose ps

# ดู logs
docker-compose logs -f master-db
docker-compose logs -f replica-one
docker-compose logs -f replica-two
```

### 2. รันแอพพลิเคชัน Go

```bash
# ติดตั้ง dependencies
go mod tidy

# รันโปรแกรม
go run main.go
```

### 3. เข้าถึง Monitoring Dashboards

**Prometheus (Port 9090):**

- URL: http://localhost:9090
- ใช้สำหรับดู metrics และ query PromQL

**Grafana (Port 3000):**

- URL: http://localhost:3000
- Username: admin
- Password: admin
- Dashboard: PostgreSQL Monitoring Dashboard

**cAdvisor (Port 8080):**

- URL: http://localhost:8080
- ดู container metrics และ resource usage

**Node Exporter (Port 9100):**

- URL: http://localhost:9100/metrics
- ดู system metrics (CPU, Memory, Disk)

### 4. ตรวจสอบการทำงาน

**เชื่อมต่อ Master Database:**

```bash
docker exec -it sharding-golang-master-db-1 psql -U postgres -d postgres
```

**เชื่อมต่อ Replica:**

```bash
# Replica 1
docker exec -it sharding-golang-replica-one-1 psql -U postgres -d postgres

# Replica 2
docker exec -it sharding-golang-replica-two-1 psql -U postgres -d postgres
```

**ตรวจสอบ Partitions:**

```sql
-- ดูจำนวนข้อมูลในแต่ละ partition
SELECT 'tbl_users_p0' as partition, count(*) FROM tbl_users_p0
UNION ALL
SELECT 'tbl_users_p1' as partition, count(*) FROM tbl_users_p1
UNION ALL
SELECT 'tbl_users_p2' as partition, count(*) FROM tbl_users_p2
UNION ALL
SELECT 'tbl_users_p3' as partition, count(*) FROM tbl_users_p3;

-- ดูข้อมูลใน partition ที่มีข้อมูล
SELECT * FROM tbl_users_p0;
SELECT * FROM tbl_users_p1;
SELECT * FROM tbl_users_p2;
SELECT * FROM tbl_users_p3;
```

**ตรวจสอบ Replication Status:**

```sql
-- ใน Master
SELECT * FROM pg_stat_replication;

-- ใน Replica
SELECT * FROM pg_stat_wal_receiver;
```

**ตรวจสอบ Monitoring Metrics:**

```bash
# ตรวจสอบ Prometheus targets
curl http://localhost:9090/api/v1/targets

# ตรวจสอบ PostgreSQL metrics
curl http://localhost:9090/api/v1/query?query=pg_up

# ตรวจสอบ database connections
curl http://localhost:9090/api/v1/query?query=pg_stat_database_numbackends
```

## 📊 การทำงานของ Hash Partitioning

PostgreSQL จะใช้ hash function กับ UUID และทำ modulus 4 เพื่อกำหนดว่าข้อมูลจะไปอยู่ partition ไหน:

hash(uuid) % 4 = 0 → tbl_users_p0
hash(uuid) % 4 = 1 → tbl_users_p1
hash(uuid) % 4 = 2 → tbl_users_p2
hash(uuid) % 4 = 3 → tbl_users_p3

## ⚡ ข้อดีของสถาปัตยกรรมนี้

1. **Load Distribution**: ข้อมูลกระจายอย่างสม่ำเสมอใน 4 partitions
2. **Read Scalability**: อ่านข้อมูลจาก 2 replicas ลดโหลดจาก master
3. **High Availability**: ถ้า replica ตัวหนึ่งล้ม ยังอ่านจากอีกตัวได้
4. **Automatic Failover**: GORM DB Resolver จะจัดการการเลือก replica อัตโนมัติ
5. **Comprehensive Monitoring**: ระบบ monitoring ครบถ้วนด้วย Prometheus + Grafana
6. **Real-time Metrics**: ติดตาม performance และ health ของระบบแบบ real-time
7. **Container Monitoring**: ติดตาม resource usage ของ containers
8. **Database Metrics**: ติดตาม PostgreSQL statistics และ replication status

## 🛠️ การหยุดระบบ

```bash
# หยุด containers
docker-compose down

# หยุดและลบ volumes (ข้อมูลจะหายหมด)
docker-compose down -v

# หยุดและลบ images
docker-compose down --rmi all
```

## 📝 หมายเหตุ

- ใช้ PostgreSQL 17 ซึ่งมีการเปลี่ยนแปลงการตั้งค่า replication จาก version เก่า
- UUID v7 ให้การกระจายที่ดีกว่า UUID v4 เพราะมี time component
- DBResolver Policy สามารถเปลี่ยนเป็น `RoundRobinPolicy` หรือ custom policy ได้
- Health checks ใน docker-compose ช่วยให้แน่ใจว่า replicas เริ่มหลังจาก master พร้อมแล้ว
- Prometheus เก็บ metrics ไว้ใน persistent volume เพื่อไม่ให้ข้อมูลหายเมื่อ restart
- Grafana มี auto-provisioning สำหรับ datasource และ dashboard
- PostgreSQL Exporters รองรับ custom queries ผ่าน YAML configuration
- ระบบ monitoring สามารถขยายเพิ่ม AlertManager สำหรับ alerting ได้
