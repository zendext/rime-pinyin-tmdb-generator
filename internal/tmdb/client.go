package tmdb

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/zendext/rime-pinyin-tmdb-generator/internal/store"
)

var errExportNotFound = errors.New("TMDb daily export not found")

type Client struct {
	BaseURL   string
	APIKey    string
	Languages []string
	HTTP      *http.Client

	BootstrapMode   string
	PopularSources  []PopularSource
	ExportBaseURL   string
	ExportDate      string
	StorePath       string
	RequestInterval time.Duration
	MaxItems        int
	Logf            func(format string, args ...any)
}

type PopularSource struct {
	Path  string
	Pages int
}

func (c *Client) FetchSeries(ctx context.Context, since time.Time) ([]SeriesRecord, error) {
	if strings.TrimSpace(c.APIKey) == "" {
		return nil, fmt.Errorf("TMDb API key is required")
	}
	mode := strings.TrimSpace(c.BootstrapMode)
	if mode == "full" {
		return c.fetchFullBootstrap(ctx, since)
	}
	if mode == "" || mode == "popular" {
		return c.fetchPopularBootstrap(ctx, since)
	}
	return nil, fmt.Errorf("unsupported bootstrap mode %q", mode)
}

func (c *Client) fetchPopularBootstrap(ctx context.Context, since time.Time) ([]SeriesRecord, error) {
	if strings.TrimSpace(c.StorePath) == "" {
		return c.fetchDiscoverOrChanges(ctx, since)
	}
	db, err := store.Open(c.StorePath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	items, err := c.fetchCommonListItems(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[int]bool)
	c.logf("popular bootstrap fetched_items=%d unique_ids=%d", len(items), uniqueItemCount(items))
	processed := 0
	total := uniqueItemCount(items)
	for _, item := range items {
		if item.ID <= 0 || seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		record, err := c.fetchSeriesTranslationsByID(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		now := time.Now().UTC()
		if err := db.UpsertSeries(store.Series{
			ID:         item.ID,
			Popularity: item.Popularity,
			Titles:     titlesFromRecord(record),
			FetchedAt:  now,
			LastSeenAt: now,
		}); err != nil {
			return nil, err
		}
		processed++
		if processed%50 == 0 {
			c.logf("popular translations progress processed=%d total=%d", processed, total)
		}
		c.wait(ctx)
	}
	count, err := db.SeriesCount()
	if err != nil {
		return nil, err
	}
	c.logf("popular bootstrap completed processed=%d store_series=%d", processed, count)
	return c.recordsFromStore(db)
}

func (c *Client) fetchDiscoverOrChanges(ctx context.Context, since time.Time) ([]SeriesRecord, error) {
	if since.IsZero() {
		return c.fetchDiscoverPages(ctx)
	}
	ids, err := c.fetchChangedSeriesIDs(ctx, since)
	if err != nil {
		return nil, err
	}
	records := make([]SeriesRecord, 0, len(ids))
	for _, id := range ids {
		record, err := c.fetchSeriesByID(ctx, id)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func (c *Client) fetchFullBootstrap(ctx context.Context, since time.Time) ([]SeriesRecord, error) {
	if strings.TrimSpace(c.StorePath) == "" {
		return nil, fmt.Errorf("TMDb full bootstrap requires a store path")
	}
	db, err := store.Open(c.StorePath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	for _, exportDate := range c.exportDates() {
		cursor, completed, err := db.Bootstrap(exportDate)
		if err != nil {
			return nil, err
		}
		c.logf("full bootstrap export=%s cursor=%d completed=%t", exportDate, cursor, completed)
		if !completed {
			if err := c.consumeExport(ctx, db, exportDate, cursor); err != nil {
				if errors.Is(err, errExportNotFound) && c.ExportDate == "" {
					continue
				}
				return nil, err
			}
		} else if !since.IsZero() {
			c.logf("full incremental since=%s", since.UTC().Format(time.RFC3339))
			if err := c.refreshChangedSeries(ctx, db, since); err != nil {
				return nil, err
			}
		}
		return c.recordsFromStore(db)
	}
	return nil, errExportNotFound
}

func (c *Client) refreshChangedSeries(ctx context.Context, db *store.DB, since time.Time) error {
	ids, err := c.fetchChangedSeriesIDs(ctx, since)
	if err != nil {
		return err
	}
	c.logf("full incremental changed_ids=%d", len(ids))
	for _, id := range ids {
		record, err := c.fetchSeriesTranslationsByID(ctx, id)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if err := db.UpsertSeries(store.Series{
			ID:         id,
			Titles:     titlesFromRecord(record),
			FetchedAt:  now,
			LastSeenAt: now,
		}); err != nil {
			return err
		}
		c.wait(ctx)
	}
	return nil
}

func (c *Client) consumeExport(ctx context.Context, db *store.DB, exportDate string, cursor int) error {
	c.logf("full bootstrap download export=%s", exportDate)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.exportBaseURL(), "/")+"/tv_series_ids_"+exportDate+".json.gz", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusForbidden {
		return errExportNotFound
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("TMDb export %s failed: %s: %s", exportDate, resp.Status, strings.TrimSpace(string(data)))
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return err
	}
	defer gz.Close()
	scanner := bufio.NewScanner(gz)
	processedItems := 0
	skippedItems := 0
	lastCursor := cursor
	persistedCursor := cursor
	for offset := 0; scanner.Scan(); offset++ {
		if offset < cursor {
			continue
		}
		var item exportSeries
		if err := json.Unmarshal(scanner.Bytes(), &item); err != nil {
			return err
		}
		nextCursor := offset + 1
		lastCursor = nextCursor
		if item.Adult || item.ID <= 0 {
			skippedItems++
			if err := db.SetBootstrap(exportDate, nextCursor, false); err != nil {
				return err
			}
			persistedCursor = nextCursor
			if skippedItems%100 == 0 || skippedItems == 1 {
				c.logf("full bootstrap skipped export=%s cursor=%d skipped=%d", exportDate, nextCursor, skippedItems)
			}
			continue
		}
		if c.MaxItems > 0 && processedItems >= c.MaxItems {
			c.logf("full bootstrap paused export=%s cursor=%d processed=%d skipped=%d reason=max_items", exportDate, persistedCursor, processedItems, skippedItems)
			return nil
		}
		record, err := c.fetchSeriesTranslationsByID(ctx, item.ID)
		if err != nil {
			return err
		}
		if err := db.UpsertSeries(store.Series{
			ID:         item.ID,
			Popularity: item.Popularity,
			Titles:     titlesFromRecord(record),
			FetchedAt:  time.Now().UTC(),
			LastSeenAt: time.Now().UTC(),
		}); err != nil {
			return err
		}
		if err := db.SetBootstrap(exportDate, nextCursor, false); err != nil {
			return err
		}
		persistedCursor = nextCursor
		processedItems++
		if processedItems%100 == 0 {
			c.logf("full bootstrap progress export=%s cursor=%d processed=%d", exportDate, nextCursor, processedItems)
		}
		c.wait(ctx)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	c.logf("full bootstrap completed export=%s cursor=%d processed=%d skipped=%d", exportDate, lastCursor, processedItems, skippedItems)
	return db.SetBootstrap(exportDate, lastCursor, true)
}

func (c *Client) fetchSeriesTranslationsByID(ctx context.Context, id int) (SeriesRecord, error) {
	var resp translationsResponse
	if err := c.request(ctx, http.MethodGet, fmt.Sprintf("/tv/%d/translations", id), nil, &resp); err != nil {
		return SeriesRecord{}, err
	}
	record := SeriesRecord{ID: id}
	attachTranslationResponse(&record, resp, c.Languages)
	return record, nil
}

func (c *Client) recordsFromStore(db *store.DB) ([]SeriesRecord, error) {
	rows, err := db.AllSeries()
	if err != nil {
		return nil, err
	}
	records := make([]SeriesRecord, 0, len(rows))
	for _, row := range rows {
		record := SeriesRecord{ID: row.ID, Translations: map[string]Translation{}}
		for language, title := range row.Titles {
			record.Translations[language] = Translation{Name: title}
		}
		records = append(records, record)
	}
	return records, nil
}

func (c *Client) fetchDiscoverPages(ctx context.Context) ([]SeriesRecord, error) {
	seen := make(map[int]bool)
	var records []SeriesRecord
	for page := 1; page <= 1; page++ {
		q := url.Values{
			"include_adult":                []string{"false"},
			"include_null_first_air_dates": []string{"false"},
			"language":                     []string{c.primaryLanguage()},
			"page":                         []string{strconv.Itoa(page)},
			"sort_by":                      []string{"popularity.desc"},
		}
		var resp discoverResponse
		if err := c.request(ctx, http.MethodGet, "/discover/tv", q, &resp); err != nil {
			return nil, err
		}
		for _, item := range resp.Results {
			if item.ID <= 0 || seen[item.ID] {
				continue
			}
			seen[item.ID] = true
			record := item.toRecord()
			c.attachTranslations(ctx, &record)
			records = append(records, record)
		}
		if resp.TotalPages > 0 && page >= resp.TotalPages {
			break
		}
	}
	return records, nil
}

func (c *Client) fetchCommonListItems(ctx context.Context) ([]seriesAPIRecord, error) {
	var items []seriesAPIRecord
	c.logf("popular bootstrap start sources=%d", len(c.PopularSources))
	for _, source := range c.PopularSources {
		for page := 1; page <= source.Pages; page++ {
			q := url.Values{
				"language": []string{c.primaryLanguage()},
				"page":     []string{strconv.Itoa(page)},
			}
			var resp discoverResponse
			if err := c.request(ctx, http.MethodGet, source.Path, q, &resp); err != nil {
				return nil, err
			}
			c.logf("popular list source=%s page=%d items=%d", source.Path, page, len(resp.Results))
			items = append(items, resp.Results...)
			if resp.TotalPages > 0 && page >= resp.TotalPages {
				break
			}
		}
	}
	return items, nil
}

func (c *Client) fetchChangedSeriesIDs(ctx context.Context, since time.Time) ([]int, error) {
	seen := make(map[int]bool)
	var ids []int
	for _, window := range changeWindows(since, time.Now().UTC()) {
		for page := 1; page <= 1; page++ {
			q := url.Values{
				"start_date": []string{window.start.Format("2006-01-02")},
				"end_date":   []string{window.end.Format("2006-01-02")},
				"page":       []string{strconv.Itoa(page)},
			}
			var resp changesResponse
			if err := c.request(ctx, http.MethodGet, "/tv/changes", q, &resp); err != nil {
				return nil, err
			}
			for _, item := range resp.Results {
				if item.ID > 0 && !seen[item.ID] {
					seen[item.ID] = true
					ids = append(ids, item.ID)
				}
			}
			if resp.TotalPages > 0 && page >= resp.TotalPages {
				break
			}
		}
	}
	return ids, nil
}

func (c *Client) fetchSeriesByID(ctx context.Context, id int) (SeriesRecord, error) {
	q := url.Values{"language": []string{c.primaryLanguage()}}
	var resp seriesAPIRecord
	if err := c.request(ctx, http.MethodGet, fmt.Sprintf("/tv/%d", id), q, &resp); err != nil {
		return SeriesRecord{}, err
	}
	record := resp.toRecord()
	c.attachTranslations(ctx, &record)
	return record, nil
}

func (c *Client) attachTranslations(ctx context.Context, record *SeriesRecord) {
	if record.Translations == nil {
		record.Translations = make(map[string]Translation)
	}
	var resp translationsResponse
	if err := c.request(ctx, http.MethodGet, fmt.Sprintf("/tv/%d/translations", record.ID), nil, &resp); err != nil {
		return
	}
	attachTranslationResponse(record, resp, c.Languages)
}

func (c *Client) request(ctx context.Context, method, path string, q url.Values, out any) error {
	if q == nil {
		q = url.Values{}
	}
	q.Set("api_key", c.APIKey)
	base := strings.TrimRight(c.baseURL(), "/")
	u, err := url.Parse(base + path)
	if err != nil {
		return err
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, method, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.doWithRetry(ctx, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("TMDb %s %s failed: %s: %s", method, path, resp.Status, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

func (c *Client) doWithRetry(ctx context.Context, req *http.Request) (*http.Response, error) {
	var last *http.Response
	for attempt := 0; attempt < 5; attempt++ {
		if last != nil {
			last.Body.Close()
		}
		resp, err := c.httpClient().Do(req.Clone(ctx))
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusTooManyRequests {
			return resp, nil
		}
		last = resp
		wait, ok := retryAfter(resp.Header.Get("Retry-After"))
		if !ok {
			wait = time.Duration(1<<attempt) * time.Second
		}
		select {
		case <-ctx.Done():
			resp.Body.Close()
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
	return last, nil
}

func (c *Client) baseURL() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return "https://api.themoviedb.org/3"
}

func (c *Client) primaryLanguage() string {
	for _, language := range c.Languages {
		if language = strings.TrimSpace(language); language != "" {
			return language
		}
	}
	return "zh-CN"
}

func (c *Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 60 * time.Second}
}

func (c *Client) exportBaseURL() string {
	if c.ExportBaseURL != "" {
		return c.ExportBaseURL
	}
	return "https://files.tmdb.org/p/exports"
}

func (c *Client) exportDates() []string {
	if c.ExportDate != "" {
		return []string{c.ExportDate}
	}
	now := time.Now().UTC()
	return []string{
		now.Format("01_02_2006"),
		now.AddDate(0, 0, -1).Format("01_02_2006"),
		now.AddDate(0, 0, -2).Format("01_02_2006"),
	}
}

func (c *Client) wait(ctx context.Context) {
	interval := c.RequestInterval
	if interval <= 0 {
		return
	}
	select {
	case <-ctx.Done():
	case <-time.After(interval):
	}
}

func (c *Client) logf(format string, args ...any) {
	if c.Logf != nil {
		c.Logf(format, args...)
	}
}

type discoverResponse struct {
	Page       int               `json:"page"`
	Results    []seriesAPIRecord `json:"results"`
	TotalPages int               `json:"total_pages"`
}

type changesResponse struct {
	Page    int `json:"page"`
	Results []struct {
		ID int `json:"id"`
	} `json:"results"`
	TotalPages int `json:"total_pages"`
}

type translationsResponse struct {
	ID           int `json:"id"`
	Translations []struct {
		ISO31661 string `json:"iso_3166_1"`
		ISO6391  string `json:"iso_639_1"`
		Name     string `json:"name"`
		Data     struct {
			Name string `json:"name"`
		} `json:"data"`
	} `json:"translations"`
}

type seriesAPIRecord struct {
	ID           int     `json:"id"`
	Name         string  `json:"name"`
	OriginalName string  `json:"original_name"`
	Popularity   float64 `json:"popularity"`
}

func (r seriesAPIRecord) toRecord() SeriesRecord {
	name := strings.TrimSpace(r.Name)
	if name == "" {
		name = strings.TrimSpace(r.OriginalName)
	}
	record := SeriesRecord{
		ID:   r.ID,
		Name: name,
	}
	if originalName := strings.TrimSpace(r.OriginalName); originalName != "" && originalName != name {
		record.Aliases = append(record.Aliases, Alias{Name: originalName})
	}
	return record
}

func normalTMDBLanguage(language, country string) string {
	language = strings.ToLower(strings.TrimSpace(language))
	country = strings.ToUpper(strings.TrimSpace(country))
	if language == "" {
		return ""
	}
	if country == "" {
		return language
	}
	return language + "-" + country
}

type exportSeries struct {
	ID         int     `json:"id"`
	Popularity float64 `json:"popularity"`
	Adult      bool    `json:"adult"`
}

func attachTranslationResponse(record *SeriesRecord, resp translationsResponse, languages []string) {
	if record.Translations == nil {
		record.Translations = make(map[string]Translation)
	}
	allowed := languageSet(languages)
	for _, item := range resp.Translations {
		language := normalTMDBLanguage(item.ISO6391, item.ISO31661)
		if language == "" || (len(allowed) > 0 && !allowed[normalLang(language)]) {
			continue
		}
		name := strings.TrimSpace(item.Data.Name)
		if name == "" {
			name = strings.TrimSpace(item.Name)
		}
		if name != "" {
			record.Translations[language] = Translation{Name: name}
		}
	}
}

func titlesFromRecord(record SeriesRecord) map[string]string {
	titles := make(map[string]string, len(record.Translations))
	for language, translation := range record.Translations {
		if title := strings.TrimSpace(translation.Name); title != "" {
			titles[language] = title
		}
	}
	return titles
}

func uniqueItemCount(items []seriesAPIRecord) int {
	seen := make(map[int]bool)
	for _, item := range items {
		if item.ID > 0 {
			seen[item.ID] = true
		}
	}
	return len(seen)
}

func retryAfter(value string) (time.Duration, bool) {
	if value == "" {
		return 0, false
	}
	seconds, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || seconds < 0 {
		return 0, false
	}
	return time.Duration(seconds) * time.Second, true
}

type changeWindow struct {
	start time.Time
	end   time.Time
}

func changeWindows(start, end time.Time) []changeWindow {
	start = dateOnly(start.UTC())
	end = dateOnly(end.UTC())
	if start.After(end) {
		return []changeWindow{{start: end, end: end}}
	}
	var out []changeWindow
	for current := start; !current.After(end); {
		windowEnd := current.AddDate(0, 0, 14)
		if windowEnd.After(end) {
			windowEnd = end
		}
		out = append(out, changeWindow{start: current, end: windowEnd})
		current = windowEnd.AddDate(0, 0, 1)
	}
	return out
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
