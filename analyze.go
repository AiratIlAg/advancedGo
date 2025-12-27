package main

import (
	"path/filepath"
	"sync"
)

// функции для тестов и бенчмарков
func AnalyzeSequential(files []string, analyzers []Analyzer) ([]FileAnalysisResult, error) {
	var results []FileAnalysisResult

	for _, path := range files {
		content, size, err := readFileContent(path)
		if err != nil {
			continue
		}

		var analysisResults []AnalysisResult
		for _, analyzer := range analyzers {
			analysisResults = append(analysisResults, analyzer.Analyze(content))
		}

		results = append(results, FileAnalysisResult{
			FileName: filepath.Base(path),
			Size:     size,
			Results:  analysisResults,
		})
	}
	return results, nil
}

func AnalyzeParallel(files []string, analyzers []Analyzer, workers int) ([]FileAnalysisResult, error) {
	filePaths := make(chan string)
	results := make(chan FileAnalysisResult)

	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range filePaths {
				content, size, err := readFileContent(path)
				if err != nil {
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
		}()
	}

	go func() {
		for _, f := range files {
			filePaths <- f
		}
		close(filePaths)
		wg.Wait()
		close(results)
	}()

	var out []FileAnalysisResult
	for r := range results {
		out = append(out, r)
	}
	return out, nil
}
