package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nwaples/rardecode/v2"
)

var errPDFRendererUnavailable = errors.New("PDF renderer unavailable")

type progressFunc func(int, string)

func (a *App) importPages(ctx context.Context, extension, source, destination, comicID string, progress progressFunc) ([]extractedPage, error) {
	switch extension {
	case ".cbz":
		return a.extractCBZProgress(source, destination, comicID, progress)
	case ".cbr":
		return a.extractCBRProgress(source, destination, comicID, progress)
	case ".pdf":
		return a.renderPDF(ctx, source, destination, comicID, progress)
	default:
		return nil, errors.New("Unsupported comic format.")
	}
}

type stagedPage struct {
	original string
	path     string
}

func (a *App) extractCBR(archivePath, destination, comicID string) ([]extractedPage, error) {
	return a.extractCBRProgress(archivePath, destination, comicID, func(int, string) {})
}

func (a *App) extractCBRProgress(archivePath, destination, comicID string, progress progressFunc) ([]extractedPage, error) {
	rr, err := rardecode.OpenReader(archivePath, rardecode.MaxDictionarySize(a.config.MaxFile))
	if err != nil {
		return nil, errors.New("The uploaded file is not a valid RAR archive.")
	}
	defer rr.Close()
	staging := destination + "-staging"
	if err := os.MkdirAll(staging, 0o750); err != nil {
		return nil, fmt.Errorf("create page storage: %w", err)
	}
	defer os.RemoveAll(staging)

	var candidates []stagedPage
	var total int64
	entries := 0
	for {
		header, err := rr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, errors.New("The RAR archive is malformed or uses an unsupported feature.")
		}
		entries++
		if entries > a.config.MaxEntries {
			return nil, errors.New("The archive contains too many entries.")
		}
		clean, ok := safeArchivePath(header.Name)
		if !ok {
			return nil, errors.New("The archive contains an unsafe path.")
		}
		if header.Encrypted || header.HeaderEncrypted {
			return nil, errors.New("Encrypted archives are not supported.")
		}
		if header.IsDir || ignoredEntry(clean) || !supportedExtension(filepath.Ext(clean)) {
			continue
		}
		if !header.UnKnownSize && header.UnPackedSize > a.config.MaxFile {
			return nil, errors.New("An archive entry is too large.")
		}
		path := filepath.Join(staging, fmt.Sprintf("%04d%s", len(candidates)+1, strings.ToLower(filepath.Ext(clean))))
		written, err := copyLimitedFile(path, rr, a.config.MaxFile)
		if err != nil {
			return nil, errors.New("An archive entry could not be extracted.")
		}
		total += written
		if total > a.config.MaxExtracted {
			return nil, errors.New("The extracted archive is too large.")
		}
		candidates = append(candidates, stagedPage{original: clean, path: path})
		progress(5+70*entries/max(1, a.config.MaxEntries), fmt.Sprintf("extracting page %d", len(candidates)))
	}
	if len(candidates) == 0 {
		return nil, errors.New("The archive does not contain supported page images.")
	}
	sort.SliceStable(candidates, func(i, j int) bool { return naturalLess(candidates[i].original, candidates[j].original) })
	if err := os.MkdirAll(destination, 0o750); err != nil {
		return nil, fmt.Errorf("create page storage: %w", err)
	}
	pages := make([]extractedPage, 0, len(candidates))
	for index, candidate := range candidates {
		ext := strings.ToLower(filepath.Ext(candidate.path))
		name := fmt.Sprintf("%04d%s", index+1, ext)
		output := filepath.Join(destination, name)
		if err := os.Rename(candidate.path, output); err != nil {
			return nil, errors.New("An extracted page could not be stored.")
		}
		page, err := inspectPage(output, candidate.original, index+1, filepath.Join("comics", comicID, "pages", name))
		if err != nil {
			return nil, err
		}
		pages = append(pages, page)
		progress(75+15*(index+1)/len(candidates), fmt.Sprintf("inspecting page %d of %d", index+1, len(candidates)))
	}
	return pages, nil
}

func (a *App) renderPDF(ctx context.Context, pdfPath, destination, comicID string, progress progressFunc) ([]extractedPage, error) {
	renderer, err := exec.LookPath("pdftocairo")
	if err != nil {
		return nil, errPDFRendererUnavailable
	}
	file, err := os.Open(pdfPath)
	if err != nil {
		return nil, errors.New("The PDF could not be opened.")
	}
	magic := make([]byte, 5)
	_, readErr := io.ReadFull(bufio.NewReader(file), magic)
	file.Close()
	if readErr != nil || string(magic) != "%PDF-" {
		return nil, errors.New("The uploaded file is not a valid PDF.")
	}
	if err := os.MkdirAll(destination, 0o750); err != nil {
		return nil, fmt.Errorf("create page storage: %w", err)
	}
	renderCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	prefix := filepath.Join(destination, "rendered")
	pageCount := pdfPageCount(renderCtx, pdfPath)
	var output []byte
	if pageCount > 0 && pageCount <= a.config.MaxEntries {
		for page := 1; page <= pageCount; page++ {
			progress(5+80*(page-1)/pageCount, fmt.Sprintf("rendering page %d of %d", page, pageCount))
			command := exec.CommandContext(renderCtx, renderer, "-jpeg", "-singlefile", "-f", strconv.Itoa(page), "-l", strconv.Itoa(page), "-jpegopt", "quality=90", "-r", "160", pdfPath, fmt.Sprintf("%s-%d", prefix, page))
			pageOutput, commandErr := command.CombinedOutput()
			output = append(output, pageOutput...)
			if commandErr != nil {
				err = commandErr
				break
			}
		}
	} else {
		progress(10, "rendering PDF")
		command := exec.CommandContext(renderCtx, renderer, "-jpeg", "-jpegopt", "quality=90", "-r", "160", pdfPath, prefix)
		output, err = command.CombinedOutput()
	}
	if errors.Is(renderCtx.Err(), context.DeadlineExceeded) {
		return nil, errors.New("PDF rendering exceeded the processing time limit.")
	}
	if err != nil {
		a.logger.Warn("PDF rendering failed", "error", err, "output", truncate(string(output), 500))
		return nil, errors.New("The PDF is malformed, encrypted, or could not be rendered.")
	}
	matches, err := filepath.Glob(prefix + "-*.jpg")
	if err != nil || len(matches) == 0 {
		return nil, errors.New("The PDF does not contain renderable pages.")
	}
	if len(matches) > a.config.MaxEntries {
		return nil, errors.New("The PDF contains too many pages.")
	}
	sort.SliceStable(matches, func(i, j int) bool { return naturalLess(matches[i], matches[j]) })
	var total int64
	pages := make([]extractedPage, 0, len(matches))
	for index, rendered := range matches {
		info, err := os.Stat(rendered)
		if err != nil || info.Size() > a.config.MaxFile {
			return nil, errors.New("A rendered PDF page is too large.")
		}
		total += info.Size()
		if total > a.config.MaxExtracted {
			return nil, errors.New("The rendered PDF is too large.")
		}
		name := fmt.Sprintf("%04d.jpg", index+1)
		outputPath := filepath.Join(destination, name)
		if err := os.Rename(rendered, outputPath); err != nil {
			return nil, errors.New("A rendered PDF page could not be stored.")
		}
		page, err := inspectPage(outputPath, fmt.Sprintf("PDF page %d", index+1), index+1, filepath.Join("comics", comicID, "pages", name))
		if err != nil {
			return nil, err
		}
		pages = append(pages, page)
		progress(85+5*(index+1)/len(matches), fmt.Sprintf("inspecting page %d of %d", index+1, len(matches)))
	}
	return pages, nil
}

func pdfPageCount(ctx context.Context, path string) int {
	pdfinfo, err := exec.LookPath("pdfinfo")
	if err != nil {
		return 0
	}
	output, err := exec.CommandContext(ctx, pdfinfo, path).Output()
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(output), "\n") {
		if key, value, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(key), "Pages") {
			pages, _ := strconv.Atoi(strings.TrimSpace(value))
			return pages
		}
	}
	return 0
}

func copyLimitedFile(path string, source io.Reader, limit int64) (int64, error) {
	output, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return 0, err
	}
	written, copyErr := io.Copy(output, io.LimitReader(source, limit+1))
	closeErr := output.Close()
	if copyErr != nil {
		return written, copyErr
	}
	if closeErr != nil {
		return written, closeErr
	}
	if written > limit {
		return written, errors.New("file exceeds limit")
	}
	return written, nil
}

func inspectPage(path, original string, number int, storedPath string) (extractedPage, error) {
	file, err := os.Open(path)
	if err != nil {
		return extractedPage{}, errors.New("An extracted image could not be opened.")
	}
	config, format, decodeErr := image.DecodeConfig(file)
	file.Close()
	if decodeErr != nil {
		return extractedPage{}, fmt.Errorf("%s is not a valid image", original)
	}
	mediaType := map[string]string{"jpeg": "image/jpeg", "png": "image/png", "gif": "image/gif"}[format]
	if mediaType == "" {
		return extractedPage{}, fmt.Errorf("%s uses an unsupported image format", original)
	}
	return extractedPage{Number: number, Width: config.Width, Height: config.Height, MediaType: mediaType, path: storedPath}, nil
}

func truncate(value string, maximum int) string {
	value = strings.TrimSpace(value)
	if len(value) <= maximum {
		return value
	}
	return value[:maximum]
}
