package main

import (
	"fmt"
	"log"
	"os"

	"recallix/internal/config"
	"recallix/internal/repository"
)

func main() {
	// 加载配置
	cfg := config.Load()

	// 连接数据库
	db, err := repository.NewDB(cfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	// 执行迁移
	log.Println("Starting database migration...")
	if err := repository.RunMigration(db); err != nil {
		log.Fatalf("Migration failed: %v", err)
	}

	log.Println("Migration completed successfully!")
	fmt.Println("Database migration completed.")
	os.Exit(0)
}
