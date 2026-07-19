package api

import (
	"archive/zip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"openreader/backend/models"
)

type bookGroupContractRow struct {
	Key         string `json:"key"`
	Kind        string `json:"kind"`
	Semantic    string `json:"semantic"`
	CategoryID  *uint  `json:"categoryId"`
	Name        string `json:"name"`
	DefaultName string `json:"defaultName"`
	Show        bool   `json:"show"`
	SortOrder   int    `json:"sortOrder"`
	Assignable  bool   `json:"assignable"`
	Deletable   bool   `json:"deletable"`
}

func TestBookGroupsProjectFourPersistedBuiltInsAndUserCategories(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	var owner models.User
	if err := server.db.Where("username = ?", "testuser").First(&owner).Error; err != nil {
		t.Fatal(err)
	}
	category := models.Category{UserID: owner.ID, Name: "自定义分组", Show: true, SortOrder: 20}
	if err := server.db.Create(&category).Error; err != nil {
		t.Fatal(err)
	}

	groups := getBookGroupContractRows(t, router, auth)
	assertBookGroupContractKeys(t, groups, []string{
		"builtin:all", "builtin:local", "builtin:audio", "builtin:ungrouped", "category:" + strconv.FormatUint(uint64(category.ID), 10),
	})
	defaults := []struct {
		key, semantic, name string
		order               int
	}{
		{"builtin:all", "all", "全部", -10},
		{"builtin:local", "local", "本地", -9},
		{"builtin:audio", "audio", "音频", -8},
		{"builtin:ungrouped", "ungrouped", "未分组", -7},
	}
	for index, expected := range defaults {
		row := groups[index]
		if row.Key != expected.key || row.Kind != "builtin" || row.Semantic != expected.semantic || row.Name != expected.name || row.DefaultName != expected.name || row.SortOrder != expected.order || !row.Show || row.Assignable || row.Deletable || row.CategoryID != nil {
			t.Fatalf("unexpected built-in %s: %+v", expected.key, row)
		}
	}
	custom := groups[len(groups)-1]
	if custom.Kind != "category" || custom.Semantic != "category" || custom.CategoryID == nil || *custom.CategoryID != category.ID || !custom.Assignable || !custom.Deletable || custom.DefaultName != "" {
		t.Fatalf("unexpected category projection: %+v", custom)
	}

	updated := bookGroupContractRequest(t, router, auth, http.MethodPut, "/api/book-groups/all", `{"name":"所有藏书","show":false}`)
	if updated.Code != http.StatusOK {
		t.Fatalf("update built-in: status=%d body=%s", updated.Code, updated.Body.String())
	}
	var all bookGroupContractRow
	if err := json.Unmarshal(updated.Body.Bytes(), &all); err != nil {
		t.Fatal(err)
	}
	if all.Key != "builtin:all" || all.Name != "所有藏书" || all.Show || all.DefaultName != "全部" {
		t.Fatalf("built-in update was not persisted: %+v", all)
	}
	groups = getBookGroupContractRows(t, router, auth)
	if groups[0].Name != "所有藏书" || groups[0].Show {
		t.Fatalf("subsequent projection lost built-in update: %+v", groups[0])
	}

	for path, body := range map[string]string{
		"/api/book-groups/all":     `{"name":"  "}`,
		"/api/book-groups/missing": `{"show":false}`,
		"/api/book-groups/local":   `{}`,
	} {
		response := bookGroupContractRequest(t, router, auth, http.MethodPut, path, body)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("invalid built-in update %s: status=%d body=%s", path, response.Code, response.Body.String())
		}
	}

	otherAuth, other := registerBookGroupContractUser(t, router, server, "groupother")
	otherGroups := getBookGroupContractRows(t, router, otherAuth)
	if other.ID == owner.ID || otherGroups[0].Name != "全部" || !otherGroups[0].Show || len(otherGroups) != 4 {
		t.Fatalf("book groups leaked between users: owner=%d other=%d groups=%+v", owner.ID, other.ID, otherGroups)
	}
}

func TestBookGroupsMixedReorderIsCompleteAtomicAndAppendsNewCategories(t *testing.T) {
	router, server := setupTestServer(t)
	auth := authHeader(t, router)
	var owner models.User
	if err := server.db.Where("username = ?", "testuser").First(&owner).Error; err != nil {
		t.Fatal(err)
	}
	first := models.Category{UserID: owner.ID, Name: "第一分组", Show: true, SortOrder: 10}
	second := models.Category{UserID: owner.ID, Name: "第二分组", Show: true, SortOrder: 20}
	if err := server.db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}
	if err := server.db.Create(&second).Error; err != nil {
		t.Fatal(err)
	}
	firstKey := "category:" + strconv.FormatUint(uint64(first.ID), 10)
	secondKey := "category:" + strconv.FormatUint(uint64(second.ID), 10)
	wanted := []string{secondKey, "builtin:audio", "builtin:all", firstKey, "builtin:local", "builtin:ungrouped"}
	response := bookGroupContractRequest(t, router, auth, http.MethodPut, "/api/book-groups/reorder", `{"keys":["`+strings.Join(wanted, `","`)+`"]}`)
	if response.Code != http.StatusOK {
		t.Fatalf("mixed reorder: status=%d body=%s", response.Code, response.Body.String())
	}
	var reordered []bookGroupContractRow
	if err := json.Unmarshal(response.Body.Bytes(), &reordered); err != nil {
		t.Fatal(err)
	}
	assertBookGroupContractKeys(t, reordered, wanted)
	for index, row := range reordered {
		if row.SortOrder != (index+1)*10 {
			t.Fatalf("mixed order %s=%d, want %d", row.Key, row.SortOrder, (index+1)*10)
		}
	}

	before := append([]string(nil), wanted...)
	invalidBodies := []string{
		`{"keys":["builtin:all"]}`,
		`{"keys":["` + strings.Join(append([]string(nil), wanted[:len(wanted)-1]...), `","`) + `"]}`,
		`{"keys":["` + strings.Join(append(append([]string(nil), wanted...), wanted[0]), `","`) + `"]}`,
		`{"keys":["` + strings.Join(append(append([]string(nil), wanted...), "category:999999"), `","`) + `"]}`,
	}
	for _, body := range invalidBodies {
		invalid := bookGroupContractRequest(t, router, auth, http.MethodPut, "/api/book-groups/reorder", body)
		if invalid.Code != http.StatusBadRequest {
			t.Fatalf("invalid mixed reorder: status=%d body=%s request=%s", invalid.Code, invalid.Body.String(), body)
		}
		assertBookGroupContractKeys(t, getBookGroupContractRows(t, router, auth), before)
	}

	created := bookGroupContractRequest(t, router, auth, http.MethodPost, "/api/categories", `{"name":"最后新增"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create category after mixed reorder: status=%d body=%s", created.Code, created.Body.String())
	}
	var newCategory models.Category
	if err := json.Unmarshal(created.Body.Bytes(), &newCategory); err != nil {
		t.Fatal(err)
	}
	groups := getBookGroupContractRows(t, router, auth)
	if groups[len(groups)-1].Key != "category:"+strconv.FormatUint(uint64(newCategory.ID), 10) || groups[len(groups)-1].SortOrder <= reordered[len(reordered)-1].SortOrder {
		t.Fatalf("new category was not appended after unified max: %+v", groups)
	}
}

func TestBookGroupBackupRoundTripAndReaderDevMaskRestore(t *testing.T) {
	sourceRouter, sourceServer := setupTestServer(t)
	sourceAuth := authHeader(t, sourceRouter)
	var sourceUser models.User
	if err := sourceServer.db.Where("username = ?", "testuser").First(&sourceUser).Error; err != nil {
		t.Fatal(err)
	}
	categoryA := models.Category{UserID: sourceUser.ID, Name: "备份甲", Show: true, SortOrder: 10}
	categoryB := models.Category{UserID: sourceUser.ID, Name: "备份乙", Show: false, SortOrder: 20}
	if err := sourceServer.db.Create(&categoryA).Error; err != nil {
		t.Fatal(err)
	}
	if err := sourceServer.db.Create(&categoryB).Error; err != nil {
		t.Fatal(err)
	}
	book := models.Book{UserID: sourceUser.ID, Title: "分组备份书", URL: "https://backup.example/book", Type: 0}
	if err := sourceServer.db.Create(&book).Error; err != nil {
		t.Fatal(err)
	}
	for _, categoryID := range []uint{categoryA.ID, categoryB.ID} {
		if err := sourceServer.db.Create(&models.BookCategory{UserID: sourceUser.ID, BookID: book.ID, CategoryID: categoryID}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if response := bookGroupContractRequest(t, sourceRouter, sourceAuth, http.MethodPut, "/api/book-groups/audio", `{"name":"有声书","show":false}`); response.Code != http.StatusOK {
		t.Fatalf("prepare built-in backup state: %d %s", response.Code, response.Body.String())
	}
	current := getBookGroupContractRows(t, sourceRouter, sourceAuth)
	keys := make([]string, 0, len(current))
	for _, row := range current {
		keys = append(keys, row.Key)
	}
	keys = append(keys[4:], keys[:4]...)
	if response := bookGroupContractRequest(t, sourceRouter, sourceAuth, http.MethodPut, "/api/book-groups/reorder", `{"keys":["`+strings.Join(keys, `","`)+`"]}`); response.Code != http.StatusOK {
		t.Fatalf("prepare backup order: %d %s", response.Code, response.Body.String())
	}

	backupPath, err := sourceServer.backupSvc.RunNowForUser(sourceUser.ID, sourceUser.Username)
	if err != nil {
		t.Fatal(err)
	}
	entries := readBookGroupBackupEntries(t, backupPath)
	bookGroupJSON, ok := entries["bookGroup.json"]
	if !ok {
		t.Fatalf("backup missing bookGroup.json; entries=%v", bookGroupBackupEntryKeys(entries))
	}
	var exportedGroups []struct {
		GroupID    int    `json:"groupId"`
		GroupName  string `json:"groupName"`
		Order      int    `json:"order"`
		Show       bool   `json:"show"`
		CategoryID *uint  `json:"categoryId"`
		Key        string `json:"key"`
	}
	if err := json.Unmarshal(bookGroupJSON, &exportedGroups); err != nil {
		t.Fatal(err)
	}
	if len(exportedGroups) != 6 {
		t.Fatalf("unexpected exported groups: %+v", exportedGroups)
	}
	maskByName := map[string]int{}
	for _, row := range exportedGroups {
		if row.GroupID > 0 {
			if row.GroupID&(row.GroupID-1) != 0 || row.CategoryID == nil || row.Key == "" {
				t.Fatalf("custom backup group is not a portable power-of-two mapping: %+v", row)
			}
			maskByName[row.GroupName] = row.GroupID
		}
	}
	var exportedBooks []struct {
		Title         string   `json:"title"`
		Group         int      `json:"group"`
		CategoryNames []string `json:"categoryNames"`
	}
	if err := json.Unmarshal(entries["bookshelf.json"], &exportedBooks); err != nil {
		t.Fatal(err)
	}
	if len(exportedBooks) != 1 || exportedBooks[0].Group != maskByName["备份甲"]|maskByName["备份乙"] || len(exportedBooks[0].CategoryNames) != 2 {
		t.Fatalf("bookshelf/group mapping does not match bookGroup.json: books=%+v masks=%+v", exportedBooks, maskByName)
	}

	archive, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatal(err)
	}
	destinationRouter, destinationServer := setupTestServer(t)
	destinationAuth := authHeader(t, destinationRouter)
	var destinationUser models.User
	if err := destinationServer.db.Where("username = ?", "testuser").First(&destinationUser).Error; err != nil {
		t.Fatal(err)
	}
	result, err := destinationServer.restoreLegadoBackupData(archive, destinationUser.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restoreResultCount(result, "bookGroups") != 6 {
		t.Fatalf("bookGroups restore count=%#v result=%+v", result["bookGroups"], result)
	}
	restoredGroups := getBookGroupContractRows(t, destinationRouter, destinationAuth)
	if restoredGroups[0].Name != "备份甲" || restoredGroups[1].Name != "备份乙" {
		t.Fatalf("unified order was not restored: %+v", restoredGroups)
	}
	audio := findBookGroupContractRow(t, restoredGroups, "builtin:audio")
	if audio.Name != "有声书" || audio.Show {
		t.Fatalf("built-in state was not restored: %+v", audio)
	}
	assertRestoredBookCategoryNames(t, destinationServer, destinationUser.ID, "分组备份书", []string{"备份甲", "备份乙"})

	readerDevArchive := makeBackupRestoreZIP(t, map[string]string{
		"bookGroup.json": `[
			{"groupId":-1,"groupName":"全部藏书","order":3,"show":false},
			{"groupId":-2,"groupName":"本地","order":4,"show":true},
			{"groupId":-3,"groupName":"音频","order":5,"show":true},
			{"groupId":-4,"groupName":"未分组","order":6,"show":true},
			{"groupId":1,"groupName":"上游甲","order":1,"show":true},
			{"groupId":2,"groupName":"上游乙","order":2,"show":false}
		]`,
		"bookshelf.json": `[{"name":"上游分组书","bookUrl":"https://reader-dev.example/book","group":3}]`,
	})
	readerDevRouter, readerDevServer := setupTestServer(t)
	readerDevAuth := authHeader(t, readerDevRouter)
	var readerDevUser models.User
	if err := readerDevServer.db.Where("username = ?", "testuser").First(&readerDevUser).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := readerDevServer.restoreLegadoBackupData(readerDevArchive, readerDevUser.ID); err != nil {
		t.Fatal(err)
	}
	readerDevGroups := getBookGroupContractRows(t, readerDevRouter, readerDevAuth)
	assertBookGroupContractKeys(t, readerDevGroups, []string{
		"category:" + strconv.FormatUint(uint64(findCategoryByName(t, readerDevServer, readerDevUser.ID, "上游甲").ID), 10),
		"category:" + strconv.FormatUint(uint64(findCategoryByName(t, readerDevServer, readerDevUser.ID, "上游乙").ID), 10),
		"builtin:all", "builtin:local", "builtin:audio", "builtin:ungrouped",
	})
	if findBookGroupContractRow(t, readerDevGroups, "builtin:all").Name != "全部藏书" || findBookGroupContractRow(t, readerDevGroups, "builtin:all").Show {
		t.Fatalf("reader-dev built-in state was not restored: %+v", readerDevGroups)
	}
	assertRestoredBookCategoryNames(t, readerDevServer, readerDevUser.ID, "上游分组书", []string{"上游甲", "上游乙"})
}

func TestBookGroupPreferencesAreDeletedWithUserData(t *testing.T) {
	router, server := setupTestServer(t)
	_ = authHeader(t, router)
	auth, user := registerBookGroupContractUser(t, router, server, "groupdelete")
	_ = getBookGroupContractRows(t, router, auth)
	var before int64
	if err := server.db.Raw("SELECT COUNT(*) FROM book_group_preferences WHERE user_id = ?", user.ID).Scan(&before).Error; err != nil {
		t.Fatal(err)
	}
	if before != 4 {
		t.Fatalf("expected four preference rows before delete, got %d", before)
	}
	if _, _, err := server.deleteUserData([]uint{user.ID}, 0); err != nil {
		t.Fatal(err)
	}
	var after int64
	if err := server.db.Raw("SELECT COUNT(*) FROM book_group_preferences WHERE user_id = ?", user.ID).Scan(&after).Error; err != nil {
		t.Fatal(err)
	}
	if after != 0 {
		t.Fatalf("deleted user left %d book group preference rows", after)
	}
}

func TestPortableBackupRecognizesBookGroupLogicalEntry(t *testing.T) {
	if !portableLogicalEntryName("bookgroup.json") {
		t.Fatal("portable backup allowlist rejected bookGroup.json after name normalization")
	}
}

func getBookGroupContractRows(t *testing.T, router *gin.Engine, auth string) []bookGroupContractRow {
	t.Helper()
	response := bookGroupContractRequest(t, router, auth, http.MethodGet, "/api/book-groups", "")
	if response.Code != http.StatusOK {
		t.Fatalf("get book groups: status=%d body=%s", response.Code, response.Body.String())
	}
	var rows []bookGroupContractRow
	if err := json.Unmarshal(response.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

func bookGroupContractRequest(t *testing.T, router *gin.Engine, auth string, method string, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Authorization", auth)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func registerBookGroupContractUser(t *testing.T, router *gin.Engine, server *Server, username string) (string, models.User) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/register", strings.NewReader(`{"username":"`+username+`","password":"password8"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("register %s: status=%d body=%s", username, response.Code, response.Body.String())
	}
	var payload struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	var user models.User
	if err := server.db.Where("username = ?", username).First(&user).Error; err != nil {
		t.Fatal(err)
	}
	return "Bearer " + payload.Token, user
}

func assertBookGroupContractKeys(t *testing.T, rows []bookGroupContractRow, expected []string) {
	t.Helper()
	actual := make([]string, 0, len(rows))
	for _, row := range rows {
		actual = append(actual, row.Key)
	}
	if strings.Join(actual, "|") != strings.Join(expected, "|") {
		t.Fatalf("book group keys=%v, want %v", actual, expected)
	}
}

func findBookGroupContractRow(t *testing.T, rows []bookGroupContractRow, key string) bookGroupContractRow {
	t.Helper()
	for _, row := range rows {
		if row.Key == key {
			return row
		}
	}
	t.Fatalf("book group %s not found in %+v", key, rows)
	return bookGroupContractRow{}
}

func readBookGroupBackupEntries(t *testing.T, path string) map[string][]byte {
	t.Helper()
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	entries := make(map[string][]byte, len(reader.File))
	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		entries[file.Name] = data
	}
	return entries
}

func bookGroupBackupEntryKeys(values map[string][]byte) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func assertRestoredBookCategoryNames(t *testing.T, server *Server, userID uint, title string, expected []string) {
	t.Helper()
	var book models.Book
	if err := server.db.Where("user_id = ? AND title = ?", userID, title).First(&book).Error; err != nil {
		t.Fatal(err)
	}
	var names []string
	if err := server.db.Model(&models.Category{}).
		Select("categories.name").
		Joins("JOIN book_categories ON book_categories.category_id = categories.id").
		Where("book_categories.user_id = ? AND book_categories.book_id = ?", userID, book.ID).
		Order("categories.sort_order asc, categories.id asc").
		Pluck("categories.name", &names).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Join(names, "|") != strings.Join(expected, "|") {
		t.Fatalf("restored categories for %s=%v, want %v", title, names, expected)
	}
}

func findCategoryByName(t *testing.T, server *Server, userID uint, name string) models.Category {
	t.Helper()
	var category models.Category
	if err := server.db.Where("user_id = ? AND name = ?", userID, name).First(&category).Error; err != nil {
		t.Fatal(err)
	}
	return category
}
