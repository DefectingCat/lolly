package gjson

// Default configuration values (matching lua-cjson defaults)
const (
	defaultEncodeSparseConvert  = false
	defaultEncodeSparseRatio    = 2
	defaultEncodeSparseSafe     = 10
	defaultEncodeMaxDepth       = 1000
	defaultDecodeMaxDepth       = 1000
	defaultEncodeNumberPrecision = 14
	defaultEncodeKeepBuffer     = true
)

// sparseArrayConfig controls sparse array handling during encoding.
type sparseArrayConfig struct {
	convert bool // true: convert to object, false: raise error
	ratio   int  // sparse ratio threshold
	safe    int  // safe threshold (arrays smaller than this are not checked)
}

// Config holds all configuration options for a GJSON instance.
type Config struct {
	encodeSparseArray    sparseArrayConfig
	encodeMaxDepth       int
	decodeMaxDepth       int
	encodeNumberPrecision int
	encodeKeepBuffer     bool
}

// defaultConfig returns a new Config with default values.
func defaultConfig() *Config {
	return &Config{
		encodeSparseArray: sparseArrayConfig{
			convert: defaultEncodeSparseConvert,
			ratio:   defaultEncodeSparseRatio,
			safe:    defaultEncodeSparseSafe,
		},
		encodeMaxDepth:       defaultEncodeMaxDepth,
		decodeMaxDepth:       defaultDecodeMaxDepth,
		encodeNumberPrecision: defaultEncodeNumberPrecision,
		encodeKeepBuffer:     defaultEncodeKeepBuffer,
	}
}

// Clone returns a deep copy of the config.
func (c *Config) Clone() *Config {
	return &Config{
		encodeSparseArray: sparseArrayConfig{
			convert: c.encodeSparseArray.convert,
			ratio:   c.encodeSparseArray.ratio,
			safe:    c.encodeSparseArray.safe,
		},
		encodeMaxDepth:       c.encodeMaxDepth,
		decodeMaxDepth:       c.decodeMaxDepth,
		encodeNumberPrecision: c.encodeNumberPrecision,
		encodeKeepBuffer:     c.encodeKeepBuffer,
	}
}
