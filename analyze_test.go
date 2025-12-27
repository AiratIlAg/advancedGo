package main

import (
	"os"
	"strings"
	"testing"
)

// тесты и бенчмарки
func createTempFile(t testing.TB, content string) string {
	t.Helper()

	f, err := os.CreateTemp("", "*.txt") //создаём файл в системной временной директории
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(content) //проверяем записалось или нет
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	return f.Name() //полный путь к файлу
}

func TestAnalyzeSequential(t *testing.T) {
	file := createTempFile(t, "hello world\nhello go")
	defer os.Remove(file)

	analyzers := []Analyzer{
		WordCountAnalyzer{},
		LineCountAnalyzer{},
	}

	results, err := AnalyzeSequential([]string{file}, analyzers)
	if err != nil {
		t.Fatal(err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	for _, res := range results[0].Results {
		switch res.NameAnalyzer {
		case "word_count":
			if res.Data.(int) != 4 {
				t.Errorf("expected 4 words")
			}
		case "line_count":
			if res.Data.(int) != 2 {
				t.Errorf("expected 2 lines")
			}
		}
	}
}

func benchmarkFiles(b *testing.B, count int, content string) []string {
	b.Helper()

	var files []string
	for i := 0; i < count; i++ {
		f := createTempFile(b, content)
		files = append(files, f)
	}

	b.Cleanup(func() { //для удаления временных файлов
		for _, f := range files {
			os.Remove(f)
		}
	})
	return files
}

func BenchmarkSequential(b *testing.B) {
	files := benchmarkFiles(
		b,
		50,
		strings.Repeat("hello world\n", 1000),
	)

	analyzers := []Analyzer{
		WordCountAnalyzer{},
		LineCountAnalyzer{},
		MostFrequentWordsAnalyzer{},
	}

	b.ResetTimer() //исключение из измерений всё что было до цикла
	for i := 0; i < b.N; i++ {
		AnalyzeSequential(files, analyzers)
	}
}

func BenchmarkParallel(b *testing.B) {
	files := benchmarkFiles(
		b,
		50,
		strings.Repeat("hello world\n", 1000),
	)

	analyzers := []Analyzer{
		WordCountAnalyzer{},
		LineCountAnalyzer{},
		MostFrequentWordsAnalyzer{},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		AnalyzeParallel(files, analyzers, 8)
	}
}
