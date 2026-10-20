Self-hosted fonts (no CDN at runtime).

NotoSans-var.woff2         Noto Sans, from github.com/google/fonts ofl/notosans/NotoSans[wdth,wght].ttf
NotoSansSinhala-var.woff2  Noto Sans Sinhala, from github.com/google/fonts ofl/notosanssinhala/NotoSansSinhala[wdth,wght].ttf

Both were processed with fontTools: subset (Noto Sans: Latin-1, general punctuation, arrows,
currency; Noto Sans Sinhala: the Sinhala block, Sinhala archaic numbers, ZWJ/ZWNJ, dotted circle,
with all OpenType layout features kept so conjuncts, yansaya and rakaransaya shape correctly),
width axis pinned to 100, weight axis kept 400-700, saved as WOFF2.

Licence: SIL Open Font License 1.1 - see OFL-NotoSans.txt and OFL-NotoSansSinhala.txt.
