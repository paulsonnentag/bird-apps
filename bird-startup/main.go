// Bird Startup: on install, downloads a library of bird photos (iNaturalist)
// and songs (xeno-canto) to disk. On every sign-in it picks a random bird from
// that local library, sets the photo as the desktop wallpaper and plays the
// song. No network is used at startup.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// The xeno-canto API key is injected at build time (see build.sh), lightly
// obfuscated as hex(key XOR pad) plus a random hex pad, so it doesn't appear
// as a plain string in the exe. This only deters casual inspection.
var (
	xcKeyData = ""
	xcKeyPad  = ""
)

var xenoCantoKey = decodeKey(xcKeyData, xcKeyPad)

func decodeKey(data, pad string) string {
	d, err1 := hex.DecodeString(data)
	p, err2 := hex.DecodeString(pad)
	if err1 != nil || err2 != nil || len(p) < len(d) {
		return ""
	}
	for i := range d {
		d[i] ^= p[i]
	}
	return string(d)
}

const (
	appName      = "BirdStartup"
	userAgent    = "BirdStartup/1.0 (desktop wallpaper app)"
	maxSoundTime = 20 * time.Second // startup sound length
	maxSongBytes = 1 << 20          // first 1MB of each recording: >=20s even at 320kbps
)

// regions the library is drawn from, in order; species already picked for an
// earlier region are skipped later. filter is extra iNaturalist query params.
var regions = []struct {
	name   string
	filter map[string]string
	count  int
}{
	{"Atlantic Puffin (always included)", map[string]string{"taxon_id": "4504"}, 1},
	{"Europe", map[string]string{"place_id": "97391"}, 80},
	{"World", nil, 70},
}

func librarySize() int {
	n := 0
	for _, r := range regions {
		n += r.count
	}
	return n
}

type Bird struct {
	Common      string `json:"common"`
	Scientific  string `json:"scientific"`
	PhotoURL    string `json:"photo_url"`
	Attribution string `json:"attribution"`
	SoundURL    string `json:"sound_url"`
	Recordist   string `json:"recordist"`
}

var httpClient = &http.Client{Timeout: 60 * time.Second}

func libraryDir() string { return filepath.Join(installDir(), "birds") }

func main() {
	uninstallFlag := flag.Bool("uninstall", false, "remove from startup and delete installed files")
	noInstall := flag.Bool("no-install", false, "don't register for startup (still downloads the library if missing)")
	startup := flag.Bool("startup", false, "sign-in mode: just show a bird from the local library")
	flag.Parse()

	_ = os.MkdirAll(installDir(), 0o755)
	if f, err := os.OpenFile(filepath.Join(installDir(), "log.txt"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); err == nil {
		log.SetOutput(io.MultiWriter(f, os.Stderr))
		defer f.Close()
	}

	if *uninstallFlag {
		if err := uninstall(); err != nil {
			messageBox("Bird Startup", "Uninstall failed: "+err.Error())
			return
		}
		messageBox("Bird Startup", "Bird Startup has been removed. Your wallpaper will stay as it is.")
		return
	}

	// Sign-in run: fully offline, just show a bird.
	if *startup {
		if len(localBirds()) == 0 { // library missing: try to recover quietly
			n, err := buildLibrary(libraryDir())
			log.Printf("library: %d birds downloaded (err=%v)", n, err)
		}
		if _, err := showRandomBird(); err != nil {
			log.Printf("showRandomBird: %v", err)
		}
		return
	}

	// Any other run (double-clicking the exe, new version or not) installs or
	// updates: replaces the installed exe, rewrites the registry entries and
	// downloads a fresh bird library.
	firstInstall := false
	if !*noInstall {
		var err error
		if firstInstall, err = install(); err != nil {
			log.Printf("install: %v", err)
		}
	}

	go messageBox("Bird Startup", fmt.Sprintf("Downloading %d birds (photos and songs). This takes a minute or two; "+
		"you'll hear the first one when it's done.", librarySize()))
	n, err := buildLibrary(libraryDir())
	log.Printf("library: %d birds downloaded (err=%v)", n, err)
	if n == 0 && len(localBirds()) == 0 {
		messageBox("Bird Startup", "Couldn't download any birds. Check your internet connection and run BirdStartup again.\n\n"+fmt.Sprint(err))
		return
	}

	bird, err := showRandomBird()
	if err != nil {
		log.Printf("showRandomBird: %v", err)
	}

	var msg string
	switch {
	case n == 0:
		msg = "Couldn't download new birds (offline?), keeping your existing library.\n\n"
	case firstInstall:
		msg = fmt.Sprintf("Bird Startup is installed with %d birds! A random one will greet you every time you sign in.\n\n", n)
	default:
		msg = fmt.Sprintf("Bird Startup is up to date, with %d fresh birds.\n\n", n)
	}
	msg += "Run BirdStartup.exe again any time to update. To remove it, use Settings > Apps > Installed apps > Bird Startup."
	if bird != nil {
		msg = "Today's bird: " + bird.Common + " (" + bird.Scientific + ")\n\n" + msg
	}
	messageBox("Bird Startup", msg)
}

// ---- startup: everything local ----

// localBirds returns the directories of fully downloaded birds.
func localBirds() []string {
	dirs, _ := filepath.Glob(filepath.Join(libraryDir(), "*", "bird.json"))
	for i, d := range dirs {
		dirs[i] = filepath.Dir(d)
	}
	return dirs
}

func showRandomBird() (*Bird, error) {
	birds := localBirds()
	if len(birds) == 0 {
		return nil, errors.New("bird library is empty")
	}
	dir := birds[rand.IntN(len(birds))]
	var b Bird
	meta, err := os.ReadFile(filepath.Join(dir, "bird.json"))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(meta, &b); err != nil {
		return nil, err
	}
	log.Printf("bird: %s (%s)", b.Common, dir)

	photo, err := os.ReadFile(filepath.Join(dir, "photo.jpg"))
	if err != nil {
		return nil, err
	}
	sw, sh := screenSize()
	wall, err := makeWallpaper(photo, sw, sh, &b)
	if err != nil {
		return nil, err
	}
	wallPath := filepath.Join(installDir(), "wallpaper.jpg")
	if err := os.WriteFile(wallPath, wall, 0o644); err != nil {
		return nil, err
	}
	if err := setWallpaper(wallPath); err != nil {
		log.Printf("setWallpaper: %v", err)
	}
	if err := playSound(filepath.Join(dir, "song.mp3"), maxSoundTime); err != nil {
		log.Printf("playSound: %v", err)
	}
	return &b, nil
}

// ---- install: build the library ----

type candidate struct {
	region                                        int
	genus, species, common, photoURL, attribution string
}

// buildLibrary downloads distinct bird species (photo + song) for each region into dir.
func buildLibrary(dir string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	tmp := dir + ".new"
	os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return 0, err
	}

	var (
		mu      sync.Mutex
		done    = make([]int, len(regions))
		lastErr error
		seen    = map[string]bool{}
		jobs    = make(chan candidate)
		wg      sync.WaitGroup
	)
	enough := func(r int) bool { mu.Lock(); defer mu.Unlock(); return done[r] >= regions[r].count }

	for w := 0; w < 6; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := range jobs {
				if enough(c.region) {
					continue
				}
				if err := fetchBird(ctx, tmp, c); err != nil {
					mu.Lock()
					lastErr = err
					mu.Unlock()
					continue
				}
				mu.Lock()
				done[c.region]++
				mu.Unlock()
			}
		}()
	}

	// Per region, walk random pages of the ~10,000 most-faved research-grade
	// bird observations there.
	for r, reg := range regions {
		for _, page := range rand.Perm(50) {
			if enough(r) || ctx.Err() != nil {
				break
			}
			cands, err := inatPage(ctx, page+1, reg.filter)
			if err != nil {
				mu.Lock()
				lastErr = err
				mu.Unlock()
				continue
			}
			for _, c := range cands {
				key := c.genus + " " + c.species
				if seen[key] || enough(r) {
					continue
				}
				seen[key] = true
				c.region = r
				jobs <- c
			}
		}
	}
	close(jobs)
	wg.Wait()

	total := 0
	for r, n := range done {
		log.Printf("library: %s: %d birds", regions[r].name, n)
		total += n
	}
	if total == 0 {
		os.RemoveAll(tmp)
		if lastErr == nil {
			lastErr = errors.New("no birds found")
		}
		return 0, lastErr
	}
	// Swap the new library in. Rename (not delete) the old one first: that
	// works even if a sign-in run is still playing a song from it.
	old := fmt.Sprintf("%s.old-%d", dir, time.Now().UnixNano())
	if err := os.Rename(dir, old); err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	if err := os.Rename(tmp, dir); err != nil {
		os.Rename(old, dir)
		return 0, err
	}
	// Best-effort cleanup of this and any earlier leftovers.
	if leftovers, _ := filepath.Glob(dir + ".old-*"); leftovers != nil {
		for _, l := range leftovers {
			os.RemoveAll(l)
		}
	}
	return total, nil
}

func fetchBird(ctx context.Context, lib string, c candidate) error {
	sound, recordist, err := findSong(ctx, c.genus, c.species)
	if err != nil {
		return err
	}
	photo, err := download(ctx, c.photoURL, 40<<20, false)
	if err != nil {
		return fmt.Errorf("photo: %w", err)
	}
	song, err := download(ctx, sound, maxSongBytes, true)
	if err != nil {
		return fmt.Errorf("song: %w", err)
	}
	d := filepath.Join(lib, strings.ToLower(c.genus+"-"+c.species))
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	b := Bird{
		Common: c.common, Scientific: c.genus + " " + c.species,
		PhotoURL: c.photoURL, Attribution: c.attribution,
		SoundURL: sound, Recordist: recordist,
	}
	meta, _ := json.MarshalIndent(b, "", "  ")
	if err := os.WriteFile(filepath.Join(d, "photo.jpg"), photo, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(d, "song.mp3"), song, 0o644); err != nil {
		return err
	}
	// bird.json last: its presence marks the bird as complete.
	return os.WriteFile(filepath.Join(d, "bird.json"), meta, 0o644)
}

// ---- iNaturalist ----

type inatResponse struct {
	Results []struct {
		Taxon struct {
			Name          string `json:"name"`
			Rank          string `json:"rank"`
			PreferredName string `json:"preferred_common_name"`
		} `json:"taxon"`
		Photos []struct {
			URL         string `json:"url"`
			Attribution string `json:"attribution"`
			Dims        struct {
				Width  int `json:"width"`
				Height int `json:"height"`
			} `json:"original_dimensions"`
		} `json:"photos"`
	} `json:"results"`
}

func inatPage(ctx context.Context, page int, filter map[string]string) ([]candidate, error) {
	q := url.Values{
		"taxon_id":      {"3"}, // Aves
		"photos":        {"true"},
		"quality_grade": {"research"},
		"order_by":      {"votes"},
		"per_page":      {"200"},
		"page":          {fmt.Sprint(page)},
		// v2 lets us request only the fields we need (full v1 pages are 20MB+).
		"fields": {"(taxon:(name:!t,rank:!t,preferred_common_name:!t)," +
			"photos:(url:!t,attribution:!t,original_dimensions:(width:!t,height:!t)))"},
	}
	for k, v := range filter {
		q.Set(k, v)
	}
	var resp inatResponse
	if err := getJSON(ctx, "https://api.inaturalist.org/v2/observations?"+q.Encode(), &resp); err != nil {
		return nil, err
	}
	var out []candidate
	for _, r := range resp.Results {
		if r.Taxon.Rank != "species" && r.Taxon.Rank != "subspecies" || len(r.Photos) == 0 {
			continue
		}
		p := r.Photos[0]
		// Landscape photos big enough to look good as a wallpaper.
		if p.Dims.Width < 1200 || float64(p.Dims.Width) < 1.2*float64(p.Dims.Height) {
			continue
		}
		parts := strings.Fields(r.Taxon.Name)
		if len(parts) < 2 {
			continue
		}
		common := r.Taxon.PreferredName
		if common == "" {
			common = parts[0] + " " + parts[1]
		}
		out = append(out, candidate{
			genus: parts[0], species: parts[1], common: common,
			photoURL:    strings.Replace(p.URL, "/square.", "/original.", 1),
			attribution: p.Attribution,
		})
	}
	rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out, nil
}

// ---- xeno-canto ----

type xcResponse struct {
	Recordings []struct {
		File     string `json:"file"`
		FileName string `json:"file-name"`
		Rec      string `json:"rec"`
		Type     string `json:"type"`
	} `json:"recordings"`
	Error   string `json:"error"`
	Message string `json:"message"`
}

func findSong(ctx context.Context, genus, species string) (fileURL, recordist string, err error) {
	if xenoCantoKey == "" {
		return "", "", errors.New("xeno-canto API key not compiled in")
	}
	// Prefer top-quality recordings, fall back to any.
	for _, extra := range []string{" q:A", ""} {
		q := url.Values{
			"query": {fmt.Sprintf("gen:%s sp:%s%s", genus, species, extra)},
			"key":   {xenoCantoKey},
		}
		var resp xcResponse
		if err := getJSON(ctx, "https://xeno-canto.org/api/3/recordings?"+q.Encode(), &resp); err != nil {
			return "", "", err
		}
		if resp.Error != "" {
			return "", "", fmt.Errorf("xeno-canto: %s %s", resp.Error, resp.Message)
		}
		// mp3 only (WAVs are huge and can't be safely truncated); prefer songs over calls.
		var mp3s, songs []int
		for i, r := range resp.Recordings {
			if strings.HasSuffix(strings.ToLower(r.FileName), ".mp3") {
				mp3s = append(mp3s, i)
				if strings.Contains(strings.ToLower(r.Type), "song") {
					songs = append(songs, i)
				}
			}
		}
		if len(songs) > 0 {
			mp3s = songs
		}
		if len(mp3s) > 0 {
			r := resp.Recordings[mp3s[rand.IntN(len(mp3s))]]
			return r.File, r.Rec, nil
		}
	}
	return "", "", fmt.Errorf("no recordings for %s %s", genus, species)
}

// ---- HTTP helpers ----

func getJSON(ctx context.Context, u string, v any) error {
	b, err := download(ctx, u, 20<<20, false)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// download fetches u. If truncateOK, bodies longer than limit are cut off
// (fine for mp3, which plays up to the cut); otherwise they're an error.
func download(ctx context.Context, u string, limit int64, truncateOK bool) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, errors.New(redact(err.Error()))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", redact(u), resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		if !truncateOK {
			return nil, fmt.Errorf("GET %s: response too large", redact(u))
		}
		b = b[:limit]
	}
	return b, nil
}

func redact(s string) string {
	if xenoCantoKey != "" {
		return strings.ReplaceAll(s, xenoCantoKey, "REDACTED")
	}
	return s
}
