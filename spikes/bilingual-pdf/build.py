"""THROWAWAY SPIKE (issue #26, Gate G0). Not production code.

Generates a bilingual Sinhala/English daily-brief HTML page in several font
variants and prints each to PDF with headless Chrome and Edge.

Usage: python build.py <fonts_dir> <out_dir>
  fonts_dir must contain NotoSansSinhala.ttf, NotoSerifSinhala.ttf (variable)
  and NotoSansSinhala-Regular.ttf, NotoSansSinhala-Bold.ttf (static).
"""
import pathlib, shutil, subprocess, sys

ZWJ = "\u200d"
HAL = "\u0dca"  # al-lakuna (virama)

def conj(c1, c2):
    """Yansaya / rakaransaya / repaya: C + al-lakuna + ZWJ + C (SLS 1134)."""
    return c1 + HAL + ZWJ + c2

def touch(c1, c2):
    """Touching letters: C + ZWJ + al-lakuna + C (SLS 1134)."""
    return c1 + ZWJ + HAL + c2

SRI = conj("ශ", "ර") + "ී"                     # ශ්‍රී
# Shaping stress strings, each labelled with the feature it exercises.
STRESS = [
    ("Yansaya", "වා" + conj("ක", "ය") + "ය", "vākyaya (sentence)"),
    ("Rakaransaya", conj("ප", "ර") + "ජාතන" + HAL + conj("ත", "ර") + "ික", "prajātantrika (democratic)"),
    ("Repaya", "ධ" + conj("ර", "ම") + "ය", "dharmaya"),
    ("Touching letters", touch("ක", "ව"), "k+va touching"),
    ("Vowel signs", "කෝ  කෞ  කෘ  කෲ  රු  රූ  ළු", "ō, au, ṛ, ṝ, special ru/rū, ḷu"),
    ("Śrī", SRI + " ලංකා", "Sri Lanka"),
    ("Stacked", "මහේස" + HAL + conj("ත", "ර") + "ාත" + HAL, "mahēstrāt (magistrate)"),
]

COURT = {
    "DC": "දිසා අධිකරණය",
    "MC": "මහේස" + HAL + conj("ත", "ර") + "ාත් අධිකරණය",
    "HC": "මහාධිකරණය",
    "PC": conj("ප", "ර") + "ාථමික අධිකරණය",
    "CA": "අභියාචනාධිකරණය",
    "SC": conj("ශ", "ර") + "ේෂ" + HAL + "ඨාධිකරණය",
}

# Fabricated hearings. Names are invented; no real people or cases.
HEARINGS = [
    ("09:30", "DC", "Colombo", "DC Colombo 12345/L",
     "සුනිල් පෙරේරා", "Sunil Perera", "කමලා " + conj("ප", "ර") + "නාන" + HAL + "දු", "Kamala Fernando",
     "සාක" + HAL + "ෂි විභාගය", "Trial — evidence of PW1", "N. Jayasinghe", "Verified", "ok"),
    ("09:30", "MC", "Gampaha", "MC Gampaha B 4521/24",
     "පොලිස් ස" + HAL + "ථානාධිපති", "OIC, Gampaha Police", "ආර්. " + conj("ධ", "ර") + "ර" + HAL + "ම", "R. Dharma",
     "වාර" + HAL + "තාව", "B report — further report", "A. Senanayake", "Unverified", "warn"),
    ("10:00", "HC", "Kandy", "HC Kandy 88/2023",
     "නීතිපති", "Hon. Attorney General", "ඩබ්. කේ. ගුණවර" + HAL + "ධන", "W. K. Gunawardena",
     "චෝදනා ගොනු කිරීම", "Indictment served", "N. Jayasinghe", "Verified", "ok"),
    ("10:00", "PC", "Kurunegala", "PC Kurunegala 66/1102",
     "එච්. එම්. බණ්ඩා", "H. M. Banda", "සෞ" + conj("ම", "ය") + " රත" + HAL + "නායක", "Saumya Ratnayake",
     "නියෝගය", "Order on s.66 dispute", "P. Wickramasinghe", "Conflict", "bad"),
    ("10:30", "CA", "Colombo", "CA (Writ) 245/2024",
     SRI + " ලංකා " + conj("ක", "ර") + "ීඩා සංගමය", "Sri Lanka Sports Assn.", "කර" + HAL + "මාන" + HAL + "ත අමාත" + HAL + "යාංශය", "Ministry of Industry",
     "ලිඛිත දේශන", "Written submissions", "A. Senanayake", "Verified", "ok"),
    ("11:00", "SC", "Colombo", "SC/APPEAL/112/2022",
     "ජයන" + HAL + "ත " + conj("ප", "ර") + "නාන" + HAL + "දු", "Jayantha Fernando", "කෞශල" + HAL + "යා පීරිස්", "Kaushalya Peiris",
     "විභාගය", "Argument — leave granted", "N. Jayasinghe", "Verified", "ok"),
]

CSS = """
@page { size: A4; margin: 14mm 12mm 16mm 12mm; }
* { box-sizing: border-box; }
body { font-family: %(family)s; font-size: 9.5pt; color: #111; margin: 0; }
header { display: flex; justify-content: space-between; border-bottom: 2px solid #1d3557; padding-bottom: 6px; }
h1 { font-size: 15pt; margin: 0; color: #1d3557; }
.sub { color: #555; font-size: 9pt; }
table { width: 100%%; border-collapse: collapse; margin-top: 10px; }
th, td { border: 1px solid #bbb; padding: 4px 5px; vertical-align: top; text-align: left; }
th { background: #eef2f7; font-size: 8.5pt; }
tr { page-break-inside: avoid; }
.si { display: block; }
.en { display: block; color: #444; font-size: 8.5pt; }
.ok { color: #14632b; } .warn { color: #8a5a00; } .bad { color: #9b1c1c; font-weight: bold; }
.stress td:nth-child(2) { font-size: 16pt; }
footer { margin-top: 12px; font-size: 8pt; color: #666; border-top: 1px solid #ccc; padding-top: 4px; }
"""

VERIFY_SI = {"Verified": "තහවුරු කළ", "Unverified": "තහවුරු නොකළ", "Conflict": "ගැටුමක්"}

def html(family_css, font_face, label):
    rows = []
    for (t, c, loc, no, p_si, p_en, r_si, r_en, purp_si, purp_en, lawyer, ver, cls) in HEARINGS:
        rows.append(f"""<tr><td>{t}</td>
<td><span class="si">{COURT[c]}, {loc}</span><span class="en">{c} {loc}</span></td>
<td>{no}</td>
<td><span class="si">{p_si} <b>එ.</b> {r_si}</span><span class="en">{p_en} v. {r_en}</span></td>
<td><span class="si">{purp_si}</span><span class="en">{purp_en}</span></td>
<td>{lawyer}</td>
<td class="{cls}"><span class="si">{VERIFY_SI[ver]}</span><span class="en">{ver}</span></td></tr>""")
    stress = "\n".join(f"<tr><td>{n}</td><td>{s}</td><td>{g}</td></tr>" for n, s, g in STRESS)
    return f"""<!doctype html><html lang="si"><head><meta charset="utf-8">
<title>Daily brief — {label}</title><style>{font_face}{CSS % {'family': family_css}}</style></head><body>
<header><div><h1>නීති මේසය · NithiDesk</h1><div class="sub">Perera &amp; Associates (fictitious) — දෛනික නඩු සාරාංශය / Daily court brief</div></div>
<div class="sub" style="text-align:right">2026 ඔක්තෝබර් 14, බදාදා<br>Wednesday 14 October 2026<br>Font: {label}</div></header>
<table><thead><tr><th>වේලාව<br>Time</th><th>අධිකරණය<br>Court</th><th>නඩු අංකය<br>Case no.</th><th>පාර්ශ්වයන්<br>Parties</th><th>අරමුණ<br>Purpose</th><th>නීතිඥ<br>Lawyer</th><th>දිනය තහවුරු<br>Date verified</th></tr></thead>
<tbody>{''.join(rows)}</tbody></table>
<h2 style="font-size:11pt;margin:14px 0 0">Shaping test strings</h2>
<table class="stress"><thead><tr><th>Feature</th><th>Rendered</th><th>Gloss</th></tr></thead><tbody>{stress}</tbody></table>
<footer>{SRI} ලංකා · පැමිණිලිකරු / Plaintiff · විත්තිකරු / Defendant · වරෙන්තු / Warrant · Fabricated sample data — spike #26.</footer>
</body></html>"""

VARIANTS = {
    "noto-sans": ("'NDSans', sans-serif",
                  "@font-face{font-family:'NDSans';src:url('fonts/NotoSansSinhala.ttf') format('truetype');font-weight:100 900;}"),
    "noto-sans-static": ("'NDStatic', sans-serif",
                         "@font-face{font-family:'NDStatic';src:url('fonts/NotoSansSinhala-Regular.ttf') format('truetype');font-weight:400;}"
                         "@font-face{font-family:'NDStatic';src:url('fonts/NotoSansSinhala-Bold.ttf') format('truetype');font-weight:700;}"),
    "noto-serif": ("'NDSerif', serif",
                   "@font-face{font-family:'NDSerif';src:url('fonts/NotoSerifSinhala.ttf') format('truetype');font-weight:100 900;}"),
    "nirmala-system": ("'Nirmala UI', sans-serif", ""),
}
ENGINES = {
    "chrome": r"C:/Program Files/Google/Chrome/Application/chrome.exe",
    "edge": r"C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe",
}

def main():
    fonts, out = pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2]).resolve()
    (out / "fonts").mkdir(parents=True, exist_ok=True)
    for f in ("NotoSansSinhala.ttf", "NotoSerifSinhala.ttf", "NotoSansSinhala-Regular.ttf", "NotoSansSinhala-Bold.ttf"):
        shutil.copy(fonts / f, out / "fonts" / f)
    (out / "strings.txt").write_text("\n".join(s for _, s, _ in STRESS) + "\n", encoding="utf-8")
    for v, (family, face) in VARIANTS.items():
        page = out / f"brief-{v}.html"
        page.write_text(html(family, face, v), encoding="utf-8")
        for e, exe in ENGINES.items():
            pdf = out / f"brief-{v}-{e}.pdf"
            subprocess.run([exe, "--headless", "--disable-gpu", "--no-pdf-header-footer",
                            f"--user-data-dir={out / ('profile-' + e)}",
                            f"--print-to-pdf={pdf}", page.as_uri()], check=True, timeout=120,
                           capture_output=True)
            print(pdf.name, pdf.stat().st_size)

if __name__ == "__main__":
    main()
