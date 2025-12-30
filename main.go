package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"stage5/feature"
	"strings"
	"sync"
)

// Интерфейсы анализаторов
type Analyzer interface {
	Analyze(content string) AnalysisResult
	Name() string
}

type AnalysisResult struct {
	NameAnalyzer string
	Data         any
}

// Структура, содержащая результаты работы всех анализаторов для файла
type FileAnalysisResult struct {
	FileName string
	Size     int64
	Results  []AnalysisResult
}

// Анализаторы количества слов, линий, общих слов
type WordCountAnalyzer struct{}
type LineCountAnalyzer struct{}
type MostFrequentWordsAnalyzer struct{}

func (w WordCountAnalyzer) Name() string {
	return "word_count"
}
func (w WordCountAnalyzer) Analyze(content string) AnalysisResult {
	words := strings.Fields(content)
	return AnalysisResult{
		NameAnalyzer: w.Name(),
		Data:         len(words),
	}
}

func (l LineCountAnalyzer) Name() string {
	return "line_count"
}
func (l LineCountAnalyzer) Analyze(content string) AnalysisResult {
	lines := strings.Count(content, "\n") + 1
	return AnalysisResult{
		NameAnalyzer: l.Name(),
		Data:         lines,
	}
}

func (m MostFrequentWordsAnalyzer) Name() string {
	return "most_frequent_words"
}
func (m MostFrequentWordsAnalyzer) Analyze(content string) AnalysisResult {
	freq := make(map[string]int)
	for _, word := range strings.Fields(content) {
		freq[strings.ToLower(word)]++
	}
	return AnalysisResult{
		NameAnalyzer: m.Name(),
		Data:         freq,
	}
}

// Поиск файлов
func dirTraversal(path, ext string, minSize, maxSize int64) ([]string, error) {
	var files []string

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	checkSize := func(info fs.FileInfo) bool {
		if minSize > 0 && info.Size() < minSize {
			return false
		}
		if maxSize > 0 && info.Size() > maxSize {
			return false
		}
		return true
	}

	if !info.IsDir() {
		if strings.HasSuffix(path, ext) && checkSize(info) {
			return []string{path}, nil
		}
		return nil, nil
	}

	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(d.Name(), ext) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if checkSize(info) {
			files = append(files, p)
		}
		return nil
	})
	return files, err
}

// Чтение файлов
func readFileContent(path string) (string, int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", 0, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", 0, err
	}
	return string(data), info.Size(), nil
}

func main() {
	var wg sync.WaitGroup

	globalMap := make(map[string]int)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	go func() {
		<-sig
		fmt.Println(" Оуществлено прерывание программы")
		cancel()
	}()

	filePaths := make(chan string, 100)
	results := make(chan FileAnalysisResult)
	filteredResults := make(chan FileAnalysisResult)

	path := flag.String("path", "", "путь к директории с текстовыми файлами (.txt) или к одному файлу")
	ext := flag.String("ext", ".txt", "расширение файлов для анализа")
	workers := flag.Int("workers", runtime.NumCPU(), "количество рабочих горутин")
	topWords := flag.Int("top-words", 0, "показать N самых часто встречающихся слов")
	minSize := flag.Int64("min-size", 0, "минимальный размер файла (байты)")
	maxSize := flag.Int64("max-size", 0, "максимальный размер файла (байты)")

	flag.Parse()

	if *path == "" {
		fmt.Println("необходимо ввести путь")
		return
	}

	files, err := dirTraversal(*path, *ext, *minSize, *maxSize)
	if err != nil {
		fmt.Println("ошибка обхода файловой системы", err)
		return
	}
	if len(files) == 0 {
		fmt.Println("файлы с расширением", *ext, "не найдены")
	}

	go func() {
		defer close(filePaths)
		for _, file := range files {
			select {
			case <-ctx.Done():
				return
			case filePaths <- file:
			}
		}

	}()

	analyzers := []Analyzer{
		WordCountAnalyzer{},
		LineCountAnalyzer{},
		MostFrequentWordsAnalyzer{},
	}

	for i := 0; i < *workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case path, ok := <-filePaths:
					if !ok {
						return
					}

					content, size, err := readFileContent(path)
					if err != nil {
						fmt.Println("ошибка обработки файла", err)
						continue
					}

					var swg sync.WaitGroup
					analysisResults := make([]AnalysisResult, len(analyzers))
					for i, analyzer := range analyzers {
						swg.Add(1)
						go func(i int, a Analyzer) {
							defer swg.Done()
							analysisResults[i] = a.Analyze(content)
						}(i, analyzer)
					}
					swg.Wait()

					results <- FileAnalysisResult{
						FileName: filepath.Base(path),
						Size:     size,
						Results:  analysisResults,
					}
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	//фильтрация на лету
	go func() {
		defer close(filteredResults)
		for res := range results {
			show := true
			for _, r := range res.Results {
				if r.NameAnalyzer == "word_count" {
					if r.Data.(int) < 2 {
						show = false
						break
					}
				}
			}
			if show {
				filteredResults <- res
			}

		}
	}()

	//Сбор результатов в карту и печать
	var totalWords, totalLines int
	for result := range filteredResults {
		fmt.Printf("Файл: %s, size: %d\n", result.FileName, result.Size)
		for _, res := range result.Results {
			switch res.NameAnalyzer {
			case "word_count":
				fmt.Println(" words:", res.Data.(int))
				totalWords += res.Data.(int)
			case "line_count":
				fmt.Println(" lines:", res.Data.(int))
				totalLines += res.Data.(int)
			case "most_frequent_words":
				freq := res.Data.(map[string]int)
				for word, count := range freq {
					globalMap[word] += count
				}
			}
		}
	}

	fmt.Printf("\nTOTAL: lines = %d, words = %d\n\n", totalLines, totalWords)

	//Поиск общих слов
	type WordCount struct {
		Word  string
		Count int
	}
	if *topWords > 0 {
		var words []WordCount
		for w, c := range globalMap {
			words = append(words, WordCount{w, c})
		}
		sort.Slice(words, func(i, j int) bool {
			return words[i].Count > words[j].Count
		})
		n := *topWords
		if n > len(words) {
			n = len(words)
		}
		for i := 0; i < n; i++ {
			fmt.Printf("Количество слов \"%s\": %d\n", words[i].Word, words[i].Count)
		}
	}
	feature.Feature()
}
