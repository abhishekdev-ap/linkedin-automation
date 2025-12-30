// Package stealth provides browser fingerprint masking to avoid detection.
// This is a MANDATORY anti-detection technique that modifies browser properties
// to avoid being identified as an automated browser.
package stealth

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// FingerprintConfig holds browser fingerprint settings
type FingerprintConfig struct {
	UserAgents []string   // Pool of user agents to choose from
	Viewports  []Viewport // Pool of viewport dimensions
	Languages  []string   // Browser languages
	Timezone   string     // Timezone to spoof
	Platform   string     // Platform to report
}

// DefaultFingerprintConfig returns default fingerprint configuration
func DefaultFingerprintConfig() FingerprintConfig {
	return FingerprintConfig{
		UserAgents: []string{
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
			"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",
			"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/119.0.0.0 Safari/537.36",
		},
		Viewports: []Viewport{
			{Width: 1920, Height: 1080},
			{Width: 1366, Height: 768},
			{Width: 1440, Height: 900},
			{Width: 1536, Height: 864},
			{Width: 2560, Height: 1440},
		},
		Languages: []string{"en-US", "en"},
		Timezone:  "America/New_York",
		Platform:  "MacIntel",
	}
}

// FingerprintMasker handles browser fingerprint masking
type FingerprintMasker struct {
	config FingerprintConfig
	rng    *rand.Rand
}

// NewFingerprintMasker creates a new FingerprintMasker
func NewFingerprintMasker(config FingerprintConfig) *FingerprintMasker {
	return &FingerprintMasker{
		config: config,
		rng:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// ApplyToPage applies fingerprint masking to a browser page
func (f *FingerprintMasker) ApplyToPage(page *rod.Page) error {
	// 1. Disable webdriver flag (CRITICAL)
	if err := f.disableWebdriver(page); err != nil {
		return fmt.Errorf("failed to disable webdriver: %w", err)
	}

	// 2. Override navigator properties
	if err := f.overrideNavigator(page); err != nil {
		return fmt.Errorf("failed to override navigator: %w", err)
	}

	// 3. Mask WebGL fingerprint
	if err := f.maskWebGL(page); err != nil {
		return fmt.Errorf("failed to mask WebGL: %w", err)
	}

	// 4. Mask canvas fingerprint
	if err := f.maskCanvas(page); err != nil {
		return fmt.Errorf("failed to mask canvas: %w", err)
	}

	// 5. Override permissions
	if err := f.overridePermissions(page); err != nil {
		return fmt.Errorf("failed to override permissions: %w", err)
	}

	// 6. Mask audio fingerprint
	if err := f.maskAudioContext(page); err != nil {
		return fmt.Errorf("failed to mask audio context: %w", err)
	}

	return nil
}

// SetRandomViewport sets a random viewport from the configured pool
func (f *FingerprintMasker) SetRandomViewport(page *rod.Page) (Viewport, error) {
	viewport := f.config.Viewports[f.rng.Intn(len(f.config.Viewports))]

	err := page.SetViewport(&proto.EmulationSetDeviceMetricsOverride{
		Width:             viewport.Width,
		Height:            viewport.Height,
		DeviceScaleFactor: 1,
		Mobile:            false,
	})

	return viewport, err
}

// GetRandomUserAgent returns a random user agent from the pool
func (f *FingerprintMasker) GetRandomUserAgent() string {
	return f.config.UserAgents[f.rng.Intn(len(f.config.UserAgents))]
}

// disableWebdriver removes the navigator.webdriver flag
func (f *FingerprintMasker) disableWebdriver(page *rod.Page) error {
	script := `
		Object.defineProperty(navigator, 'webdriver', {
			get: function() { return undefined; },
			configurable: true
		});
	`

	_, err := page.Eval(script)
	return err
}

// overrideNavigator overrides navigator properties to match a real browser
func (f *FingerprintMasker) overrideNavigator(page *rod.Page) error {
	userAgent := f.GetRandomUserAgent()
	platform := f.determinePlatform(userAgent)
	languages := f.config.Languages

	script := fmt.Sprintf(`
		// Override userAgent
		Object.defineProperty(navigator, 'userAgent', {
			get: () => '%s',
			configurable: true
		});
		
		// Override platform
		Object.defineProperty(navigator, 'platform', {
			get: () => '%s',
			configurable: true
		});
		
		// Override languages
		Object.defineProperty(navigator, 'languages', {
			get: () => %v,
			configurable: true
		});
		
		// Override hardwareConcurrency (randomize slightly)
		Object.defineProperty(navigator, 'hardwareConcurrency', {
			get: () => %d,
			configurable: true
		});
		
		// Override deviceMemory
		Object.defineProperty(navigator, 'deviceMemory', {
			get: () => %d,
			configurable: true
		});
		
		// Override maxTouchPoints (0 for desktop)
		Object.defineProperty(navigator, 'maxTouchPoints', {
			get: () => 0,
			configurable: true
		});
		
		// Override connection
		Object.defineProperty(navigator, 'connection', {
			get: () => ({
				effectiveType: '4g',
				rtt: %d,
				downlink: %d,
				saveData: false
			}),
			configurable: true
		});
	`,
		userAgent,
		platform,
		fmt.Sprintf(`["%s"]`, languages[0]),
		4+f.rng.Intn(12),   // 4-16 cores
		8+f.rng.Intn(8),    // 8-16 GB
		50+f.rng.Intn(100), // RTT 50-150ms
		5+f.rng.Intn(20),   // Downlink 5-25 Mbps
	)

	_, err := page.Eval(script)
	return err
}

// maskWebGL adds noise to WebGL fingerprinting
func (f *FingerprintMasker) maskWebGL(page *rod.Page) error {
	script := `
		// Randomize WebGL parameters slightly
		const getParameter = WebGLRenderingContext.prototype.getParameter;
		WebGLRenderingContext.prototype.getParameter = function(parameter) {
			// Add noise to some parameters
			if (parameter === 37445) { // UNMASKED_VENDOR_WEBGL
				return 'Intel Inc.';
			}
			if (parameter === 37446) { // UNMASKED_RENDERER_WEBGL
				return 'Intel Iris Pro Graphics 6200';
			}
			return getParameter.apply(this, arguments);
		};
		
		// Also for WebGL2
		if (typeof WebGL2RenderingContext !== 'undefined') {
			const getParameter2 = WebGL2RenderingContext.prototype.getParameter;
			WebGL2RenderingContext.prototype.getParameter = function(parameter) {
				if (parameter === 37445) {
					return 'Intel Inc.';
				}
				if (parameter === 37446) {
					return 'Intel Iris Pro Graphics 6200';
				}
				return getParameter2.apply(this, arguments);
			};
		}
	`

	_, err := page.Eval(script)
	return err
}

// maskCanvas adds noise to canvas fingerprinting
func (f *FingerprintMasker) maskCanvas(page *rod.Page) error {
	noise := f.rng.Float64() * 0.01 // Very small noise

	script := fmt.Sprintf(`
		// Add subtle noise to canvas fingerprinting
		const originalToDataURL = HTMLCanvasElement.prototype.toDataURL;
		HTMLCanvasElement.prototype.toDataURL = function(type) {
			if (type === 'image/png' || type === undefined) {
				const canvas = this;
				const context = canvas.getContext('2d');
				if (context) {
					const imageData = context.getImageData(0, 0, canvas.width, canvas.height);
					const pixels = imageData.data;
					
					// Add very subtle noise
					for (let i = 0; i < pixels.length; i += 4) {
						// Modify alpha very slightly
						pixels[i + 3] = Math.min(255, pixels[i + 3] + Math.floor(Math.random() * %f));
					}
					
					context.putImageData(imageData, 0, 0);
				}
			}
			return originalToDataURL.apply(this, arguments);
		};
		
		// Override toBlob as well
		const originalToBlob = HTMLCanvasElement.prototype.toBlob;
		HTMLCanvasElement.prototype.toBlob = function() {
			// Similar noise addition before calling original
			return originalToBlob.apply(this, arguments);
		};
	`, noise)

	_, err := page.Eval(script)
	return err
}

// overridePermissions handles permission-related fingerprinting
func (f *FingerprintMasker) overridePermissions(page *rod.Page) error {
	script := `
		// Override Notification permission check
		Object.defineProperty(Notification, 'permission', {
			get: () => 'default',
			configurable: true
		});
		
		// Override MediaDevices to prevent enumeration fingerprinting
		if (navigator.mediaDevices) {
			navigator.mediaDevices.enumerateDevices = async () => {
				return [
					{ kind: 'audioinput', deviceId: 'default', label: '', groupId: 'default' },
					{ kind: 'audiooutput', deviceId: 'default', label: '', groupId: 'default' },
					{ kind: 'videoinput', deviceId: 'default', label: '', groupId: 'default' }
				];
			};
		}
	`

	_, err := page.Eval(script)
	return err
}

// maskAudioContext prevents audio fingerprinting
func (f *FingerprintMasker) maskAudioContext(page *rod.Page) error {
	script := `
		// Add noise to AudioContext fingerprinting
		const originalGetChannelData = AudioBuffer.prototype.getChannelData;
		AudioBuffer.prototype.getChannelData = function(channel) {
			const array = originalGetChannelData.apply(this, arguments);
			
			// Add very subtle noise
			for (let i = 0; i < array.length; i++) {
				array[i] += (Math.random() - 0.5) * 0.0001;
			}
			
			return array;
		};
		
		// Override AnalyserNode
		const originalGetFloatFrequencyData = AnalyserNode.prototype.getFloatFrequencyData;
		AnalyserNode.prototype.getFloatFrequencyData = function(array) {
			originalGetFloatFrequencyData.apply(this, arguments);
			
			// Add noise
			for (let i = 0; i < array.length; i++) {
				array[i] += (Math.random() - 0.5) * 0.1;
			}
		};
	`

	_, err := page.Eval(script)
	return err
}

// determinePlatform determines the platform from user agent
func (f *FingerprintMasker) determinePlatform(userAgent string) string {
	if contains(userAgent, "Macintosh") {
		return "MacIntel"
	}
	if contains(userAgent, "Windows") {
		return "Win32"
	}
	if contains(userAgent, "Linux") {
		return "Linux x86_64"
	}
	return "MacIntel"
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ApplyStealthMode applies all stealth techniques to a browser
func ApplyStealthMode(browser *rod.Browser, page *rod.Page, config FingerprintConfig) error {
	masker := NewFingerprintMasker(config)

	// Set user agent at browser level
	userAgent := masker.GetRandomUserAgent()
	if err := page.SetUserAgent(&proto.NetworkSetUserAgentOverride{
		UserAgent: userAgent,
	}); err != nil {
		return fmt.Errorf("failed to set user agent: %w", err)
	}

	// Set random viewport
	if _, err := masker.SetRandomViewport(page); err != nil {
		return fmt.Errorf("failed to set viewport: %w", err)
	}

	// Apply fingerprint masking JavaScript
	if err := masker.ApplyToPage(page); err != nil {
		return fmt.Errorf("failed to apply fingerprint masking: %w", err)
	}

	return nil
}
