package middleware

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"openreader/backend/models"
)

func newActivityContractRouter(t *testing.T) (*gin.Engine, *gorm.DB, models.User) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "activity.db")), &gorm.Config{
		Logger: gormlogger.Discard,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&models.User{}); err != nil {
		t.Fatal(err)
	}
	user := models.User{Username: "activityuser", PasswordHash: "hash", Role: "user", AuthVersion: 1}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) {
		userID, err := strconv.ParseUint(c.GetHeader("X-Activity-User"), 10, 64)
		if err != nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		c.Set(userIDKey, uint(userID))
	})
	router.Use(TrackActivity(database))
	router.GET("/", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	return router, database, user
}

func performActivityContractRequest(t *testing.T, router *gin.Engine, userID uint) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-Activity-User", strconv.FormatUint(uint64(userID), 10))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("activity request = %d, want 204", response.Code)
	}
}

func TestTrackActivityRewritesLastActiveAtOnlyOnceAMinute(t *testing.T) {
	router, database, user := newActivityContractRouter(t)
	utcPlus8 := time.FixedZone("UTC+8", 8*60*60)
	utcMinus5 := time.FixedZone("UTC-5", -5*60*60)
	now := time.Now()
	// Stored values keep the offset they were written with (TrackActivity
	// writes local time, login and registration UTC), so freshness must be
	// decided by instant, not by comparing SQLite text.
	cases := []struct {
		name    string
		stored  any
		rewrite bool
	}{
		{name: "30 seconds ago in UTC", stored: now.Add(-30 * time.Second).UTC()},
		{name: "30 seconds ago in UTC+8", stored: now.Add(-30 * time.Second).In(utcPlus8)},
		{name: "30 seconds ago in UTC-5", stored: now.Add(-30 * time.Second).In(utcMinus5)},
		{name: "two minutes ago in UTC+8", stored: now.Add(-2 * time.Minute).In(utcPlus8), rewrite: true},
		{name: "two minutes ago in UTC-5", stored: now.Add(-2 * time.Minute).In(utcMinus5), rewrite: true},
		{name: "zero time", stored: time.Time{}, rewrite: true},
		{name: "NULL", stored: gorm.Expr("NULL"), rewrite: true},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if err := database.Model(&models.User{}).Where("id = ?", user.ID).
				UpdateColumn("last_active_at", testCase.stored).Error; err != nil {
				t.Fatal(err)
			}
			requestedAt := time.Now()
			performActivityContractRequest(t, router, user.ID)

			var stored []sql.NullTime
			if err := database.Model(&models.User{}).Where("id = ?", user.ID).
				Pluck("last_active_at", &stored).Error; err != nil {
				t.Fatal(err)
			}
			if len(stored) != 1 {
				t.Fatalf("users rows = %d, want 1", len(stored))
			}
			if testCase.rewrite {
				if !stored[0].Valid || stored[0].Time.Before(requestedAt) {
					t.Errorf("last_active_at stored %s = %+v after a request, want rewritten to >= %s", testCase.name, stored[0], requestedAt)
				}
				return
			}
			if want := testCase.stored.(time.Time); !stored[0].Valid || !stored[0].Time.Equal(want) {
				t.Errorf("last_active_at stored %s = %+v after a request, want it kept at %s", testCase.name, stored[0], want)
			}
		})
	}
}

func TestTrackActivityIgnoresMissingUser(t *testing.T) {
	router, database, user := newActivityContractRouter(t)
	performActivityContractRequest(t, router, user.ID+1)
	var count int64
	if err := database.Model(&models.User{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("users rows after activity for a missing user = %d, want 1", count)
	}
}
