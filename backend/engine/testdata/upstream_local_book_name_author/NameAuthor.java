import java.io.File;
import java.util.ArrayList;
import java.util.List;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

// Faithful port of reader-dev@fa22f27:
//   LocalBook.analyzeNameAuthor, BookHelp.formatBookName/formatBookAuthor,
//   AppPattern.{nameRegex,authorRegex,fileNameRegex},
//   BookController.importBookPreview file-name normalization,
//   epublib PackageDocumentMetadataReader.createAuthor + Author.toString + EpubFile.upBookInfo regex.
public class NameAuthor {
    static final Pattern[] PATTERNS = {
        Pattern.compile("(.*?)《([^《》]+)》.*?作者：(.*)"),
        Pattern.compile("(.*?)《([^《》]+)》(.*)"),
        Pattern.compile("(^)(.+) 作者：(.+)$"),
        Pattern.compile("(^)(.+) by (.+)$"),
    };
    static final Pattern NAME = Pattern.compile("\\s+作\\s*者.*|\\s+\\S+\\s+著");
    static final Pattern AUTHOR = Pattern.compile("^\\s*作\\s*者[:：\\s]+|\\s+著");
    static final Pattern FILE_NAME = Pattern.compile("[\\\\/:*?\"<>|.]");

    static String ktTrim(String s) {
        int start = 0, end = s.length();
        while (start < end && s.charAt(start) <= ' ') start++;
        while (end > start && s.charAt(end - 1) <= ' ') end--;
        return s.substring(start, end);
    }
    static String formatBookName(String v) { return ktTrim(NAME.matcher(v).replaceAll("")); }
    static String formatBookAuthor(String v) { return ktTrim(AUTHOR.matcher(v).replaceAll("")); }
    static String beforeLast(String s, String d) { int i = s.lastIndexOf(d); return i < 0 ? s : s.substring(0, i); }

    static String[] analyze(String fileName) {
        String temp = beforeLast(fileName, ".");
        for (Pattern p : PATTERNS) {
            Matcher m = p.matcher(temp);
            if (m.find()) {
                String g1 = m.group(1) == null ? "" : m.group(1);
                String g3 = m.group(3) == null ? "" : m.group(3);
                return new String[]{m.group(2), formatBookAuthor(g1 + g3)};
            }
        }
        String name = formatBookName(temp);
        String author = formatBookAuthor(temp.replace(name, ""));
        if (author.length() == temp.length()) author = "";
        return new String[]{name, author};
    }

    static String ext(String url) {
        String file = url.split("\\?", 2)[0];
        String[] parts = file.split("/", -1);
        file = parts[parts.length - 1];
        int dot = file.lastIndexOf('.');
        return dot >= 0 ? file.substring(dot + 1) : "";
    }
    static String nameExcludeExtension(String path) {
        String fileName = new File(path).getName();
        int dot = fileName.lastIndexOf('.');
        return dot != -1 ? fileName.substring(0, dot) : fileName;
    }
    static String directStoredName(String uploadName) {
        String e = ext(uploadName);
        String name = nameExcludeExtension(uploadName);
        name = FILE_NAME.matcher(name).replaceAll("");
        name = name.substring(0, Math.min(50, name.length())) + "." + e;
        return name;
    }

    static String epubAuthor(String creatorText) {
        String s = creatorText.trim();
        if (s.isEmpty()) return null;
        int space = s.lastIndexOf(' ');
        String first, last;
        if (space < 0) { first = ""; last = s; } else { first = s.substring(0, space); last = s.substring(space + 1); }
        return (last + ", " + first).replaceAll("^, |, $", "");
    }

    static String json(String v) {
        StringBuilder b = new StringBuilder("\"");
        for (char c : v.toCharArray()) {
            if (c == '"' || c == '\\') b.append('\\').append(c);
            else if (c < 0x20) b.append(String.format("\\u%04x", (int) c));
            else b.append(c);
        }
        return b.append('"').toString();
    }

    public static void main(String[] args) throws Exception {
        List<String> names = java.nio.file.Files.readAllLines(java.nio.file.Paths.get(args[0]), java.nio.charset.StandardCharsets.UTF_8);
        List<String> creators = java.nio.file.Files.readAllLines(java.nio.file.Paths.get(args[1]), java.nio.charset.StandardCharsets.UTF_8);
        StringBuilder out = new StringBuilder("{\n  \"fileNames\": [\n");
        for (int i = 0; i < names.size(); i++) {
            String n = names.get(i);
            String[] local = analyze(n);
            String stored = directStoredName(n);
            String[] direct = analyze(stored);
            out.append("    {\"fileName\": ").append(json(n))
               .append(", \"localName\": ").append(json(local[0])).append(", \"localAuthor\": ").append(json(local[1]))
               .append(", \"directStoredName\": ").append(json(stored))
               .append(", \"directName\": ").append(json(direct[0])).append(", \"directAuthor\": ").append(json(direct[1]))
               .append("}").append(i + 1 < names.size() ? ",\n" : "\n");
        }
        out.append("  ],\n  \"epubCreators\": [\n");
        for (int i = 0; i < creators.size(); i++) {
            String c = creators.get(i).replace("\\t", "\t");
            String a = epubAuthor(c);
            out.append("    {\"creator\": ").append(json(c)).append(", \"author\": ").append(a == null ? "null" : json(a))
               .append("}").append(i + 1 < creators.size() ? ",\n" : "\n");
        }
        out.append("  ]\n}\n");
        System.out.print(out);
    }
}
