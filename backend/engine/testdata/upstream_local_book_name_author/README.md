# reader-dev local book name/author fixtures

`NameAuthor.java` is a line-by-line Java port of reader-dev@fa22f27
`LocalBook.analyzeNameAuthor`, `BookHelp.formatBookName/formatBookAuthor`,
`AppPattern.{nameRegex,authorRegex,fileNameRegex}`, the file-name normalization
in `BookController.importBookPreview`, and epublib's
`PackageDocumentMetadataReader.createAuthor` + `Author.toString` as used by
`EpubFile.upBookInfo`. `upstream.json` is its output for `names.txt` and
`creators.txt`, produced offline with:

    docker run --rm --network none -v "$PWD:/w:ro" -w /w eclipse-temurin:21-jdk \
      java -Dstdout.encoding=UTF-8 NameAuthor.java names.txt creators.txt > upstream.json

`localName/localAuthor` are LocalStore/WebDAV imports (original file name);
`directName/directAuthor` are direct uploads (normalized stored file name).
