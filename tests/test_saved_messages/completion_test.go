package test_saved_messages

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	"github.com/hngprojects/telex_be/internal/config"
	"github.com/hngprojects/telex_be/internal/models"
	controller "github.com/hngprojects/telex_be/pkg/controller/savedMessages"
	"github.com/hngprojects/telex_be/pkg/repository/centrifuge"
	"github.com/hngprojects/telex_be/pkg/repository/storage"
	"github.com/hngprojects/telex_be/utility"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func completionDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	cfg := config.Setup(&utility.Logger{}, "../../app").TestDatabase
	if cfg.DB_HOST == "" {
		t.Skip("PostgreSQL test database is not configured")
	}
	dsn := url.URL{
		Scheme: "postgres", Host: net.JoinHostPort(cfg.DB_HOST, cfg.DB_PORT),
		User: url.UserPassword(cfg.USERNAME, cfg.PASSWORD), Path: cfg.DB_NAME,
		RawQuery: url.Values{"sslmode": {cfg.SSLMODE}}.Encode(),
	}
	db, err := gorm.Open(postgres.Open(dsn.String()), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.Close() })
	if err := db.AutoMigrate(&models.SavedMessage{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestSavedMessageCompletionHTTP(t *testing.T) {
	db := completionDatabase(t)
	record := models.SavedMessage{
		ID: uuid.NewString(), UserID: uuid.NewString(), OrgId: uuid.NewString(),
		ChannelsID: uuid.NewString(), ThreadID: uuid.NewString(), Type: "thread",
	}
	if err := db.Create(&record).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Unscoped().Delete(&models.SavedMessage{}, "id = ?", record.ID) })
	var publications atomic.Int32
	realtime := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		publications.Add(1)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"result":{}}`)
	}))
	t.Cleanup(realtime.Close)
	previous := centrifuge.Client.C
	centrifuge.NewCentrifugoService(&utility.Logger{}, config.Centrifuge{Url: realtime.URL})
	t.Cleanup(func() { centrifuge.Client.C = previous })
	handler := controller.Controller{Db: &storage.Database{Postgresql: db}, Validator: validator.New(), Logger: &utility.Logger{}}
	router := gin.New()
	router.PUT("/organisations/:org_id/mark-complete/:smId", func(c *gin.Context) {
		c.Set("userClaims", jwt.MapClaims{"user_id": record.UserID})
		handler.MarkCompleteSavedMessage(c)
	})
	for _, desired := range []bool{true, false, false} {
		path := fmt.Sprintf("/organisations/%s/mark-complete/%s", record.OrgId, record.ID)
		req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(fmt.Sprintf(`{"completed":%t}`, desired)))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		if response.Code != http.StatusOK {
			t.Fatalf("completed=%t: HTTP %d: %s", desired, response.Code, response.Body.String())
		}
		var stored models.SavedMessage
		if err := db.First(&stored, "id = ?", record.ID).Error; err != nil {
			t.Fatal(err)
		}
		if stored.Completed != desired {
			t.Errorf("HTTP 200 for completed=%t but database completed=%t", desired, stored.Completed)
		}
	}
	if publications.Load() != 2 {
		t.Errorf("incomplete requests should publish notifications; got %d", publications.Load())
	}
}

func TestSavedMessageCompletionTransitions(t *testing.T) {
	db := completionDatabase(t)
	for _, kind := range []string{"thread", "reply"} {
		t.Run(kind, func(t *testing.T) {
			record := models.SavedMessage{
				ID: uuid.NewString(), UserID: uuid.NewString(), OrgId: uuid.NewString(),
				ChannelsID: uuid.NewString(), ThreadID: uuid.NewString(), Type: kind,
			}
			if kind == "reply" {
				messageID := uuid.NewString()
				record.MessageID = &messageID
			}
			if err := db.Create(&record).Error; err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { db.Unscoped().Delete(&models.SavedMessage{}, "id = ?", record.ID) })
			for i, desired := range []bool{true, true, false, false, true} {
				var result models.SavedMessage
				req := models.MarkCompleteSavedMessageRequest{
					SavedMessageID: record.ID, UserId: record.UserID, OrgId: record.OrgId, Completed: &desired,
				}
				status, changed, err := result.UpdateSavedMessageCompletion(db, &utility.Logger{}, req, map[string]any{"completed": &desired})
				if err != nil || status != http.StatusOK {
					t.Fatalf("completion %t: status=%d error=%v", desired, status, err)
				}
				if changed != (i == 0 || i == 2 || i == 4) {
					t.Errorf("completion %t at request %d: changed=%t", desired, i, changed)
				}
				var stored models.SavedMessage
				if err := db.First(&stored, "id = ?", record.ID).Error; err != nil {
					t.Fatal(err)
				}
				if stored.Completed != desired {
					t.Errorf("requested completed=%t; database completed=%t", desired, stored.Completed)
				}
				if result.ID != record.ID || result.Type != kind || result.Completed != desired {
					t.Errorf("completion %t did not return the current saved message: %+v", desired, result)
				}
			}

			for _, missing := range []bool{false, true} {
				desired := false
				req := models.MarkCompleteSavedMessageRequest{SavedMessageID: record.ID, UserId: uuid.NewString(), Completed: &desired}
				if missing {
					req.SavedMessageID, req.UserId = uuid.NewString(), record.UserID
				}
				var result models.SavedMessage
				status, changed, err := result.UpdateSavedMessageCompletion(db, &utility.Logger{}, req, map[string]any{"completed": &desired})
				if status != http.StatusNotFound || err == nil {
					t.Errorf("missing=%t: status=%d error=%v", missing, status, err)
				}
				if changed {
					t.Errorf("missing=%t: completion must not be marked changed", missing)
				}
			}
			var stored models.SavedMessage
			if err := db.First(&stored, "id = ?", record.ID).Error; err != nil {
				t.Fatal(err)
			}
			if !stored.Completed {
				t.Error("unsuccessful requests changed the saved message")
			}
		})
	}
}
