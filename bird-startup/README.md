# Bird Startup (Windows)

A single `BirdStartup.exe`. Each time you sign in it sets a random bird as your
desktop wallpaper (photo from [iNaturalist](https://www.inaturalist.org)) and plays
that bird's song (from [xeno-canto](https://xeno-canto.org)) as a startup sound.

## Install

Download [`dist/BirdStartup.exe`](dist/BirdStartup.exe) and double-click `BirdStartup.exe`. On first run it:

1. Copies itself to `%LOCALAPPDATA%\BirdStartup\`. No admin rights are needed.
2. Registers itself to run at sign-in (`HKCU\...\Run`) and adds an entry under
   **Settings → Apps → Installed apps**.
3. Downloads a library of about 150 bird species, 80 European and 70 from around
   the world (a photo plus the start of a song recording for each, roughly 300 MB) into `%LOCALAPPDATA%\BirdStartup\birds\`.

After that, startup works fully offline: it picks a random bird from the local
library, crops the photo to your screen, adds the name and credits, sets it as the
wallpaper, and plays up to 20 s of the song with a fade-out.

**Updating:** run `BirdStartup.exe` again (a new download or the installed copy).
Every manual run replaces the installed exe and downloads a fresh set of birds.
The sign-in entry runs it with `--startup`, which only uses the local library.
The library always includes an Atlantic Puffin.

Flags: `--uninstall` removes the app,
or you can uninstall it from Settings → Apps. Each run writes a log to
`%LOCALAPPDATA%\BirdStartup\log.txt`.

## Build (from Linux, macOS or Windows)

Requires Go 1.24+. The xeno-canto API key is baked into the exe at build time. It
comes from `$XC_API_KEY` or `xenocanto.key` in this folder (gitignored).

```bash
./build.sh   # -> dist/BirdStartup.exe
```

The key is XOR-masked with a random pad per build, so it isn't a plain string in
the exe. That stops casual `strings` scraping but not anyone determined to get it.

To test the download and wallpaper logic on a non-Windows machine, pass the key
with an all-zero pad, which leaves it unchanged:

```bash
go run -ldflags "-X main.xcKeyData=$(tr -d '\n' < xenocanto.key | xxd -p -c0) -X main.xcKeyPad=$(head -c64 /dev/zero | xxd -p -c0)" . --no-install
```

On that path, setting the wallpaper and playing the sound are only printed.
