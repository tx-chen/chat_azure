package models

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/glebarez/go-sqlite"
	"github.com/sirupsen/logrus"
)

var GlobalDB *sql.DB

type User struct {
	ID         int
	Username   string `sql:"not null;unique" yaml:"Username"`
	Token      string `sql:"not null;unique" yaml:"Token"`
	Count      int    `sql:"not null;default:0" ` // openai tokens 用于计费
	Status     int    `sql:"not null;default:1"`  // 0 禁用, 1 普通, 2 管理员
	CreateTime time.Time
	UpdateTime time.Time
}

func InitDB(ctx context.Context) {
	var err error
	dbPath := "chat.db"
	if v, exist := os.LookupEnv("DB_ROOT"); exist {
		dbPath = filepath.Join(v, dbPath)
	} else {
		cwd, _ := os.Getwd()
		dbPath = filepath.Join(cwd, dbPath)
	}
	logrus.Infof("# DB@sqlite: [%s]", dbPath)
	GlobalDB, err = sql.Open("sqlite", dbPath)
	if err != nil {
		panic(err)
	}
	go func() {
		<-ctx.Done()
		GlobalDB.Close()
	}()
}

func AddUser(db *sql.DB, username string, token string, status int) error {
	sqlStmt := `
		INSERT INTO users (username, token, status, create_time, update_time)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`
	_, err := db.Exec(sqlStmt, username, token, status)
	if err != nil {
		return err
	}

	return nil
}

func AddCount(db *sql.DB, token string, count int) error {
	// 查找用户记录
	row := db.QueryRow(`
		SELECT id, count, update_time
		FROM users
		WHERE token = ?
	`, token)

	var id, prevCount int
	var prevUpdateTime time.Time
	if err := row.Scan(&id, &prevCount, &prevUpdateTime); err != nil {
		if err == sql.ErrNoRows {
			return errors.New("user not found")
		}
		return err
	}

	// 更新用户记录
	stmt, err := db.Prepare(`
		UPDATE users
		SET count = ?, update_time = CURRENT_TIMESTAMP
		WHERE id = ? AND count = ?
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	// logrus.Infof("id:%v prevCount:%v, updated:%v", id, prevCount, prevCount+count)

	res, err := stmt.Exec(prevCount+count, id, prevCount)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("concurrent update detected")
	}

	return nil
}

func QueryUserByToken(db *sql.DB, token string) (*User, error) {
	row := db.QueryRow(`
		SELECT id, username, token, count, status, create_time, update_time
		FROM users
		WHERE token = ?
	`, token)

	var user User
	err := row.Scan(
		&user.ID,
		&user.Username,
		&user.Token,
		&user.Count,
		&user.Status,
		&user.CreateTime,
		&user.UpdateTime,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, err
	}

	return &user, nil
}

func QueryUserByName(db *sql.DB, username string) (*User, error) {
	row := db.QueryRow(`
		SELECT id, username, token, count, status, create_time, update_time
		FROM users
		WHERE username = ?
	`, username)

	var user User
	err := row.Scan(
		&user.ID,
		&user.Username,
		&user.Token,
		&user.Count,
		&user.Status,
		&user.CreateTime,
		&user.UpdateTime,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, err
	}

	return &user, nil
}

func UpdateTokenByName(db *sql.DB, username, token string) error {
	// 查找用户记录
	row := db.QueryRow(`
		SELECT id
		FROM users
		WHERE username = ?
	`, username)

	var id int
	if err := row.Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return errors.New("user not found")
		}
		return err
	}

	// 更新用户记录
	stmt, err := db.Prepare(`
		UPDATE users
		SET token = ?
		WHERE id = ?
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	res, err := stmt.Exec(token, id)
	if err != nil {
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("concurrent update detected")
	}

	return nil
}
