# Bird Startup (Windows)

A single `BirdStartup.exe`. Each time you sign in it sets a random bird as your
desktop wallpaper (photo from [iNaturalist](https://www.inaturalist.org)) and plays
that bird's song (from [xeno-canto](https://xeno-canto.org)) as a startup sound.

## Install

Double-click `BirdStartup.exe`. On first run it:

1. Copies itself to `%LOCALAPPDATA%\BirdStartup\`. No admin rights are needed.
2. Registers itself to run at sign-in (`HKCU\...\Run`) and adds an entry under
   **Settings → Apps → Installed apps**.
3. Downloads a library of about 60 bird species (a photo plus the first 20–40 s of
   a song recording, roughly 130 MB in total) into `%LOCALAPPDATA%\BirdStartup\birds\`.

After that, startup works fully offline: it picks a random bird from the local
library, crops the photo to your screen, adds the name and credits, sets it as the
wallpaper, and plays up to 20 s of the song with a fade-out.

Flags: `--refresh` re-downloads a new set of birds. `--uninstall` removes the app,
or you can uninstall it from Settings → Apps. Each run writes a log to
`%LOCALAPPDATA%\BirdStartup\log.txt`.

## Build (from Linux, macOS or Windows)

Requires Go 1.24+. The xeno-canto API key is baked into the exe at build time. It
comes from `$XC_API_KEY` or `xenocanto.key` in this folder (gitignored).

```bash
./build.sh   # -> dist/BirdStartup.exe
```

On a non-Windows machine you can test the download and wallpaper logic with
`go run -ldflags "-X main.xenoCantoKey=$(cat xenocanto.key)" . --no-install`.
On that path, setting the wallpaper and playing the sound are only printed.
