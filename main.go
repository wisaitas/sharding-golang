package main

import (
	"fmt"
	"log"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

type User struct {
	ID        uuid.UUID `gorm:"column:id;type:uuid;primary_key"`
	FirstName string    `gorm:"column:first_name;type:varchar(255);not null"`
}

func (User) TableName() string {
	return "tbl_users"
}

func (u *User) BeforeCreate(tx *gorm.DB) (err error) {
	u.ID, err = uuid.NewV7()
	if err != nil {
		return err
	}
	return
}

var ENV struct {
	MasterDB struct {
		DBName     string `env:"NAME" envDefault:"postgres"`
		DBHost     string `env:"HOST" envDefault:"localhost"`
		DBPort     string `env:"PORT" envDefault:"5432"`
		DBUser     string `env:"USER" envDefault:"postgres"`
		DBPassword string `env:"PASSWORD" envDefault:"postgres"`
	} `envPrefix:"MASTER_DB_"`
	ReplicaOne struct {
		DBName     string `env:"NAME" envDefault:"postgres"`
		DBHost     string `env:"HOST" envDefault:"localhost"`
		DBPort     string `env:"PORT" envDefault:"5433"`
		DBUser     string `env:"USER" envDefault:"postgres"`
		DBPassword string `env:"PASSWORD" envDefault:"postgres"`
	} `envPrefix:"REPLICA_ONE_"`
	ReplicaTwo struct {
		DBName     string `env:"NAME" envDefault:"postgres"`
		DBHost     string `env:"HOST" envDefault:"localhost"`
		DBPort     string `env:"PORT" envDefault:"5434"`
		DBUser     string `env:"USER" envDefault:"postgres"`
		DBPassword string `env:"PASSWORD" envDefault:"postgres"`
	} `envPrefix:"REPLICA_TWO_"`
}

func init() {
	if err := env.Parse(&ENV); err != nil {
		log.Fatalf("error parsing environment variables: %v", err)
	}
}

func main() {
	db := connectDB()

	insertData(db)
	queryPartitions(db)
}

func GetDSN(
	dbName string,
	dbHost string,
	dbPort string,
	dbUser string,
	dbPassword string,
) string {
	return fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		dbHost,
		dbPort,
		dbUser,
		dbPassword,
		dbName,
	)
}

func connectDB() *gorm.DB {
	db, err := gorm.Open(postgres.Open(GetDSN(ENV.MasterDB.DBName, ENV.MasterDB.DBHost, ENV.MasterDB.DBPort, ENV.MasterDB.DBUser, ENV.MasterDB.DBPassword)), &gorm.Config{})
	if err != nil {
		log.Fatalf("error connecting to master database: %v", err)
	}

	db.Use(dbresolver.Register(dbresolver.Config{
		Sources: []gorm.Dialector{postgres.Open(GetDSN(ENV.MasterDB.DBName, ENV.MasterDB.DBHost, ENV.MasterDB.DBPort, ENV.MasterDB.DBUser, ENV.MasterDB.DBPassword))},
		Replicas: []gorm.Dialector{
			postgres.Open(GetDSN(ENV.ReplicaOne.DBName, ENV.ReplicaOne.DBHost, ENV.ReplicaOne.DBPort, ENV.ReplicaOne.DBUser, ENV.ReplicaOne.DBPassword)),
			postgres.Open(GetDSN(ENV.ReplicaTwo.DBName, ENV.ReplicaTwo.DBHost, ENV.ReplicaTwo.DBPort, ENV.ReplicaTwo.DBUser, ENV.ReplicaTwo.DBPassword)),
		},
		Policy: dbresolver.RandomPolicy{},
	}))

	return db
}

func insertData(db *gorm.DB) {
	users := []User{
		{FirstName: "John"},
		{FirstName: "Jane"},
		{FirstName: "Bob"},
		{FirstName: "Alice"},
		{FirstName: "Charlie"},
	}

	if err := db.Create(&users).Error; err != nil {
		log.Fatalf("error creating users: %v", err)
	}
}

func queryPartitions(db *gorm.DB) {
	fmt.Println("\n=== Partition Distribution ===")

	partitions := []string{"tbl_users_p0", "tbl_users_p1", "tbl_users_p2", "tbl_users_p3"}

	for _, partition := range partitions {
		var count int64
		db.Table(partition).Count(&count)
		fmt.Printf("Partition %s: %d records\n", partition, count)

		var users []User
		db.Table(partition).Find(&users)
		for _, user := range users {
			fmt.Printf("  - %s (ID: %s)\n", user.FirstName, user.ID)
		}
	}

	user := User{}
	if err := db.First(&user).Error; err != nil {
		log.Fatalf("error getting user: %v", err)
	}

	uuidTime := user.ID.Time()
	sec, nsec := uuidTime.UnixTime()
	createdTime := time.Unix(sec, nsec)
	fmt.Printf("User: %s (ID: %s, CreatedDate: %s)\n", user.FirstName, user.ID, createdTime.Format("2006-01-02 15:04:05"))
}
