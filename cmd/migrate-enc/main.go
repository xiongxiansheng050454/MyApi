package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"MyApi/internal/config"
	"MyApi/internal/model"
	"MyApi/internal/secret"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "path of config yaml")
	dryRun := flag.Bool("dry-run", false, "only report, do not update")
	flag.Parse()

	if err := run(*cfgPath, *dryRun); err != nil {
		fmt.Fprintln(os.Stderr, "migrate-enc:", err)
		os.Exit(1)
	}
}

func run(cfgPath string, dryRun bool) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	sec, _, err := secret.FromEnvOrFile(cfg.Security.APIKeyEncKeyFile)
	if err != nil {
		return err
	}
	if !cfg.Database.Enabled {
		return errors.New("database disabled in config")
	}
	c := cfg.Database
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.Name, c.SSLMode)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return err
	}

	var rows []model.Channel
	if err := db.Select("id", "api_key").Find(&rows).Error; err != nil {
		return err
	}

	var toEncrypt []model.Channel
	for i := range rows {
		r := &rows[i]
		if r.APIKey == "" || len(r.APIKey) >= len(secret.Prefix) && r.APIKey[:len(secret.Prefix)] == secret.Prefix {
			continue
		}
		toEncrypt = append(toEncrypt, *r)
	}
	if len(toEncrypt) == 0 {
		fmt.Println("no plaintext api_key found")
		return nil
	}
	fmt.Printf("found %d plaintext channel(s) to encrypt (dry-run=%v)\n", len(toEncrypt), dryRun)

	err = db.Transaction(func(tx *gorm.DB) error {
		for i := range toEncrypt {
			r := &toEncrypt[i]
			enc, err := sec.Encrypt(r.APIKey)
			if err != nil {
				return fmt.Errorf("encrypt channel %d: %w", r.ID, err)
			}
			if dryRun {
				fmt.Printf("[dry-run] channel %d would be encrypted\n", r.ID)
				continue
			}
			if err := tx.Model(&model.Channel{}).Where("id = ?", r.ID).Update("api_key", enc).Error; err != nil {
				return err
			}
			fmt.Printf("channel %d encrypted\n", r.ID)
		}
		return nil
	})
	return err
}
