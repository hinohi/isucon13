package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"crypto/sha512"

	"github.com/google/uuid"
	"github.com/gorilla/sessions"
	"github.com/labstack/echo-contrib/session"
	"github.com/labstack/echo/v4"
)

const (
	defaultSessionIDKey = "SESSIONID"
)

type User struct {
	ID          int    `db:"id"`
	Name        string `db:"name"`
	DisplayName string `db:"display_name"`
	Description string `db:"description"`
	// Password is hashed password.
	Password string `db:"password"`
	// CreatedAt is the created timestamp that forms an UNIX time.
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

type Session struct {
	// ID is an identifier that forms an UUIDv4.
	ID     string `db:"id"`
	UserID int    `db:"user_id"`
	// Expires is the UNIX timestamp that the sesison will be expired.
	Expires int `db:"expires"`
}

type PostUserRequest struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	// Password is non-hashed password.
	Password string `json:"password"`
}

type LoginRequest struct {
	UserName string `json:"username"`
	// Password is non-hashed password.
	Password string `json:"password"`
}

// ユーザ登録API
// POST /user
func userRegisterHandler(c echo.Context) error {
	ctx := c.Request().Context()

	req := PostUserRequest{}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "failed to decode the request body as json")
	}

	hashedPassword := sha512.Sum512([]byte(req.Password))
	user := User{
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		Password:    fmt.Sprintf("%x", hashedPassword),
	}

	tx, err := dbConn.BeginTxx(ctx, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if _, err := tx.NamedExecContext(ctx, "INSERT INTO users (name, display_name, description, password) VALUES(:name, :display_name, :description, :password)", user); err != nil {
		tx.Rollback()
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if err := tx.Commit(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusCreated, user)
}

// ユーザログインAPI
// POST /login
func loginHandler(c echo.Context) error {
	ctx := c.Request().Context()

	req := LoginRequest{}
	if err := json.NewDecoder(c.Request().Body).Decode(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "failed to decode the request body as json")
	}

	tx, err := dbConn.BeginTxx(ctx, nil)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	user := User{}
	// usernameはUNIQUEなので、whereで一意に特定できる
	if err := tx.GetContext(ctx, &user, "SELECT * FROM users WHERE name = ?", req.UserName); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	hashedPassword := fmt.Sprintf("%x", sha512.Sum512([]byte(req.Password)))
	if req.UserName != user.Name || hashedPassword != user.Password {
		return echo.NewHTTPError(http.StatusUnauthorized, "invalid username or password")
	}

	sessionEndAt := time.Now().Add(10 * time.Minute)

	sessionID := uuid.NewString()
	userSession := Session{
		ID:      sessionID,
		UserID:  user.ID,
		Expires: int(sessionEndAt.Unix()),
	}

	if _, err := tx.NamedExecContext(ctx, "INSERT INTO sessions (id, user_id, expires) VALUES(:id, :user_id, :expires)", userSession); err != nil {
		// 変更系なのでロールバックする
		tx.Rollback()
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if err := tx.Commit(); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	sess, err := session.Get(defaultSessionIDKey, c)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	sess.Options = &sessions.Options{
		MaxAge: int(600 /* 10 seconds */),
		Path:   "/",
	}
	sess.Values[defaultSessionIDKey] = userSession.ID

	if err := sess.Save(c.Request(), c.Response()); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.NoContent(http.StatusOK)
}

// ユーザ詳細API
// GET /user/:userid
func userHandler(c echo.Context) error {
	ctx := c.Request().Context()
	sess, err := session.Get(defaultSessionIDKey, c)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	sessionID, ok := sess.Values[defaultSessionIDKey]
	if !ok {
		// FIXME: エラーメッセージを検討する
		return echo.NewHTTPError(http.StatusForbidden, "")
	}

	userSession := Session{}
	err = dbConn.GetContext(ctx, &userSession, "SELECT user_id, expires FROM sessions where id = ?", sessionID.(string))
	if err != nil {
		// セッション情報が保存されていないので、Forbiddenとする
		// FIXME: エラーメッセージを検討する
		return echo.NewHTTPError(http.StatusForbidden, "")
	}

	now := time.Now()
	if now.Unix() > int64(userSession.Expires) {
		// セッションの有効期限が切れたので、もう一度ログインしてもらう
		if _, err := dbConn.NamedExecContext(ctx, "DELETE FROM sessoins WHERE id = :id", userSession); err != nil {
			// レコード削除のエラーは無視する
			c.Logger().Warn("failed to delete the session info from DB")
		}

		return echo.NewHTTPError(http.StatusUnauthorized, "session has expired")
	}

	userID := c.Param("user_id")
	user := User{}
	if err := dbConn.Get(&user, "SELECT name, display_name, description, created_at, updated_at FROM users WHERE id = ?", userID); err != nil {
		return echo.NewHTTPError(http.StatusUnauthorized, "session has expired")
	}

	return c.JSON(http.StatusOK, user)
}

// ユーザ
// XXX セッション情報返すみたいな？
// GET /user
func userSessionHandler(c echo.Context) error {
	return nil
}

// ユーザが登録しているチャンネル一覧
// GET /user/:user_id/channel
func userChannelHandler(c echo.Context) error {
	return nil
}

// チャンネル登録
// POST /user/:user_id/channel/:channelid/subscribe
func subscribeChannelHandler(c echo.Context) error {
	return nil
}

// チャンネル登録解除
// POST /user/:user_id/channel/:channelid/unsubscribe
func unsubscribeChannelHandler(c echo.Context) error {
	return nil
}

// チャンネル情報
// GET /channel/:channel_id
func channelHandler(c echo.Context) error {
	return nil
}

// チャンネル登録者数
// GET /channel/:channel_id/subscribers
func channelSubscribersHandler(c echo.Context) error {
	return nil
}

// チャンネルの動画一覧
// GET /channel/:channel_id/movie
func channelMovieHandler(c echo.Context) error {
	return nil
}

// チャンネル作成
// POST /channel
func createChannelHandler(c echo.Context) error {
	return nil
}

// チャンネル編集
// PUT /channel/:channel_id
func updateChannelHandler(c echo.Context) error {
	return nil
}

// チャンネル削除
// DELETE /channel/:channel_id
func deleteChannelHandler(c echo.Context) error {
	return nil
}
