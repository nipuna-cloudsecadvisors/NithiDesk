package briefs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"

	"github.com/nipuna-cloudsecadvisors/NithiDesk/internal/config"
)

// ErrNoBrowser means no Chromium-family browser was configured or found.
// Retrying cannot fix it, so a render job fails permanently.
var ErrNoBrowser = errors.New("briefs: no headless Chromium available")

// PDFRenderer prints HTML to PDF with headless Chromium (ADR 0007).
//
// Safety rules from ADR 0007 and spike #26, all enforced here rather than
// trusted to the template:
//   - JavaScript is off twice over: the blink setting at launch and
//     Emulation.setScriptExecutionDisabled on the page.
//   - No network: every request the page makes is intercepted with the
//     Fetch domain and failed unless it is a data: or about: URL. As defence
//     in depth, DNS resolves nothing and any request that escaped
//     interception would go to a dead local proxy.
//   - A fresh browser and profile per render, so nothing from one firm's
//     brief can survive into the next render.
//   - One render at a time per process: Chromium is memory-hungry and a PDF
//     job never needs to be fast, only finished.
//
// The sandbox stays on. If a deployment ever has to drop it, that is a
// decision for the container image, not a silent default here.
type PDFRenderer struct {
	mu      sync.Mutex
	chrome  string
	timeout time.Duration
	logger  *slog.Logger
}

// NewPDFRenderer resolves the browser once. An empty ChromePath searches the
// usual install locations.
func NewPDFRenderer(cfg config.PDF, logger *slog.Logger) *PDFRenderer {
	timeout := cfg.RenderTimeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &PDFRenderer{chrome: FindChrome(cfg.ChromePath), timeout: timeout, logger: logger}
}

// Available reports whether a browser was found.
func (r *PDFRenderer) Available() bool { return r.chrome != "" }

// FindChrome returns configured if it names an existing file, otherwise the
// first Chromium-family browser found in the standard locations, or "".
func FindChrome(configured string) string {
	if configured != "" {
		if isFile(configured) {
			return configured
		}
		return ""
	}
	var candidates []string
	switch runtime.GOOS {
	case "windows":
		for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "LocalAppData"} {
			if base := os.Getenv(env); base != "" {
				candidates = append(candidates,
					filepath.Join(base, "Google", "Chrome", "Application", "chrome.exe"),
					filepath.Join(base, "Chromium", "Application", "chrome.exe"),
					filepath.Join(base, "Microsoft", "Edge", "Application", "msedge.exe"))
			}
		}
	case "darwin":
		candidates = append(candidates,
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium")
	}
	for _, c := range candidates {
		if isFile(c) {
			return c
		}
	}
	for _, name := range []string{"headless-shell", "chromium", "chromium-browser", "google-chrome", "google-chrome-stable", "chrome"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

func isFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular()
}

// Print renders a complete HTML document to an A4 PDF. The page size and
// margins come from the document's @page rule (preferCSSPageSize).
func (r *PDFRenderer) Print(ctx context.Context, html string) ([]byte, error) {
	if r.chrome == "" {
		return nil, ErrNoBrowser
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	// The timeout bounds the whole browser's life, not just one call: when it
	// fires, chromedp kills the process.
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	opts := append(slices.Clone(chromedp.DefaultExecAllocatorOptions[:]),
		chromedp.ExecPath(r.chrome),
		chromedp.DisableGPU,
		chromedp.Flag("blink-settings", "scriptEnabled=false"),
		chromedp.Flag("host-resolver-rules", "MAP * ~NOTFOUND"),
		chromedp.ProxyServer("http://127.0.0.1:9"),
		chromedp.Flag("proxy-bypass-list", "<-loopback>"),
		chromedp.Flag("disable-component-update", true),
		chromedp.Flag("disable-domain-reliability", true),
		chromedp.Flag("no-pings", true),
		chromedp.Flag("mute-audio", true),
	)
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, opts...)
	defer cancelAlloc()
	// chromedp's own log lines can quote protocol payloads, which here are
	// brief content; they never go to the log pipeline (doc 10 §7).
	quiet := func(string, ...any) {}
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx,
		chromedp.WithLogf(quiet), chromedp.WithErrorf(quiet), chromedp.WithDebugf(quiet))
	defer cancelBrowser()

	var blocked atomic.Int64
	pdf, err := chromedp.Run(browserCtx, printAction(html, &blocked))
	if n := blocked.Load(); n > 0 {
		// A template that tries to fetch anything is a bug; the request was
		// refused, so the output is still safe, but say so.
		r.logger.Warn("pdf renderer blocked page requests", slog.Int64("count", n))
	}
	if err != nil {
		return nil, fmt.Errorf("render pdf: %w", err)
	}
	if len(pdf) < 5 || string(pdf[:5]) != "%PDF-" {
		return nil, errors.New("render pdf: browser returned no PDF")
	}
	return pdf, nil
}

// allowedURL is the whole network policy of the renderer.
func allowedURL(u string) bool {
	return strings.HasPrefix(u, "data:") || u == "about:blank" || strings.HasPrefix(u, "about:srcdoc")
}

func printAction(html string, blocked *atomic.Int64) chromedp.Action[[]byte] {
	return func(ctx context.Context, t *chromedp.Target) ([]byte, error) {
		lctx, stop := context.WithCancel(ctx)
		defer stop()

		// Subscribe before enabling interception so no paused request is
		// missed (a missed one would hang the page until the timeout).
		paused := chromedp.Events(lctx, fetch.RequestPaused)
		go func() {
			for ev, err := range paused {
				if err != nil {
					return
				}
				if ev.Request != nil && allowedURL(ev.Request.URL) {
					_, _ = cdp.Call(lctx, t, fetch.ContinueRequest, fetch.ContinueRequestParams{RequestID: ev.RequestID})
					continue
				}
				blocked.Add(1)
				_, _ = cdp.Call(lctx, t, fetch.FailRequest, fetch.FailRequestParams{
					RequestID: ev.RequestID, ErrorReason: network.ErrorReasonBlockedByClient})
			}
		}()
		if _, err := cdp.Call(ctx, t, fetch.Enable, fetch.EnableParams{
			Patterns: []*fetch.RequestPattern{{URLPattern: "*"}}}); err != nil {
			return nil, fmt.Errorf("enable request interception: %w", err)
		}
		if _, err := cdp.Call(ctx, t, emulation.SetScriptExecutionDisabled,
			emulation.SetScriptExecutionDisabledParams{Value: true}); err != nil {
			return nil, fmt.Errorf("disable scripts: %w", err)
		}

		// Load about:blank so chromedp tracks the main frame, then write the
		// document into it rather than navigating to a data: URL, which
		// Chromium caps at 2 MB; the embedded fonts alone come close to that.
		if _, err := chromedp.Navigate("about:blank")(ctx, t); err != nil {
			return nil, fmt.Errorf("open blank page: %w", err)
		}
		tree, err := cdp.Call(ctx, t, page.GetFrameTree, cdp.Empty{})
		if err != nil || tree.FrameTree == nil || tree.FrameTree.Frame == nil {
			return nil, fmt.Errorf("frame tree: %w", errors.Join(err, errors.New("no main frame")))
		}
		loaded := chromedp.Events(lctx, page.LoadEventFired)
		if _, err := cdp.Call(ctx, t, page.SetDocumentContent, page.SetDocumentContentParams{
			FrameID: tree.FrameTree.Frame.ID, HTML: html}); err != nil {
			return nil, fmt.Errorf("set document: %w", err)
		}
		// Fonts in use delay the load event, so once it fires the embedded
		// faces are decoded and the PDF cannot fall back to system fonts.
		for _, err := range loaded {
			if err != nil {
				return nil, fmt.Errorf("wait for load: %w", err)
			}
			break
		}

		return chromedp.PrintToPDF(
			chromedp.PDFPreferCSSPageSize(),
			chromedp.PDFPrintBackground(),
			chromedp.PDFStream(),
			// Page numbers only: header and footer templates are rendered
			// without the page's fonts, so they must stay ASCII. An empty
			// header stops the browser printing its own (title, URL, date).
			chromedp.PDFHeaderTemplate(`<span></span>`),
			chromedp.PDFFooterTemplate(`<div style="width:100%;font-size:7pt;text-align:right;padding:0 10mm;color:#444">`+
				`<span class="pageNumber"></span> / <span class="totalPages"></span></div>`),
		)(ctx, t)
	}
}
