package engine

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type ArchivedBook struct {
	Directory    string
	OriginalFile string
	TOCFile      string
	SourceFile   string
}

type ArchivedChapter struct {
	ID                  uint   `json:"id,omitempty"`
	URL                 string `json:"url"`
	Title               string `json:"title"`
	IsVolume            bool   `json:"isVolume"`
	BaseURL             string `json:"baseUrl"`
	BookURL             string `json:"bookUrl"`
	Index               int    `json:"index"`
	Start               int    `json:"start"`
	End                 int    `json:"end"`
	CachePath           string `json:"cachePath,omitempty"`
	ResourcePath        string `json:"resourcePath,omitempty"`
	ResourceFragment    string `json:"resourceFragment,omitempty"`
	ResourceEndFragment string `json:"resourceEndFragment,omitempty"`
}

type ArchivedBookSource struct {
	BookURL            string  `json:"bookUrl"`
	Origin             string  `json:"origin"`
	OriginName         string  `json:"originName"`
	Type               int     `json:"type"`
	Name               string  `json:"name"`
	Author             string  `json:"author"`
	Kind               *string `json:"kind"`
	CoverURL           *string `json:"coverUrl"`
	Intro              *string `json:"intro"`
	WordCount          *int    `json:"wordCount"`
	LatestChapterTitle string  `json:"latestChapterTitle"`
	TOCURL             string  `json:"tocUrl"`
	Time               int64   `json:"time"`
	Variable           *string `json:"variable"`
	OriginOrder        int     `json:"originOrder"`
	UserNameSpace      string  `json:"userNameSpace"`
}

var unsafePathChars = regexp.MustCompile(`[\\/:*?"<>|\r\n\t]+`)

func ArchiveImportedBook(libraryDir, userName, title, author, originalFilename string, data []byte) (ArchivedBook, error) {
	userName = SafeFilename(userName)
	if userName == "" {
		userName = "default"
	}
	parentDirectory := filepath.Join("data", userName)
	folderName := SafeBookFolderName(title, author)
	directoryName, err := uniqueDirectory(filepath.Join(libraryDir, parentDirectory), folderName)
	if err != nil {
		return ArchivedBook{}, err
	}
	directory := filepath.Join(parentDirectory, directoryName)
	if err := os.MkdirAll(filepath.Join(libraryDir, directory), 0o755); err != nil {
		return ArchivedBook{}, err
	}

	originalFile := filepath.Join(directory, SafeFilename(originalFilename))
	if err := os.WriteFile(filepath.Join(libraryDir, originalFile), data, 0o644); err != nil {
		return ArchivedBook{}, err
	}

	return ArchivedBook{
		Directory:    directory,
		OriginalFile: originalFile,
		TOCFile:      filepath.Join(directory, "chapters.json"),
		SourceFile:   filepath.Join(directory, "bookSource.json"),
	}, nil
}

func WriteBookSource(libraryDir string, archive ArchivedBook, source ArchivedBookSource) error {
	return writeJSON(filepath.Join(libraryDir, archive.SourceFile), []ArchivedBookSource{source})
}

func WriteChapterArchive(libraryDir string, archive ArchivedBook, chapters []ArchivedChapter) error {
	return writeJSON(filepath.Join(libraryDir, archive.TOCFile), chapters)
}

func SafeBookFolderName(title, author string) string {
	title = strings.TrimSpace(title)
	author = strings.TrimSpace(author)
	if title == "" {
		title = "未命名书籍"
	}
	return SafeFilename(title + "_" + author)
}

func SafeFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "untitled"
	}
	name = unsafePathChars.ReplaceAllString(name, "_")
	name = strings.Trim(name, " .")
	if name == "" {
		return "untitled"
	}
	if len([]rune(name)) > 120 {
		runes := []rune(name)
		name = string(runes[:120])
	}
	return name
}

func StableID(value string) string {
	sum := md5.Sum([]byte(value))
	return hex.EncodeToString(sum[:])
}

func uniqueDirectory(parentDir, folderName string) (string, error) {
	candidate := folderName
	for index := 2; ; index++ {
		_, err := os.Stat(filepath.Join(parentDir, candidate))
		if os.IsNotExist(err) {
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
		separator := "_"
		if strings.HasSuffix(folderName, "_") {
			separator = ""
		}
		candidate = fmt.Sprintf("%s%s%d", folderName, separator, index)
	}
}

func writeJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
