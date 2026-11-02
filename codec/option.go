package codec

// Option configures [Decode]. The zero set of options is a complete,
// usable configuration: options only replace defaults.
type Option func(*config)

// config is the resolved [Decode] configuration.
type config struct {
	uri string
}

// newConfig applies opts over the defaults: the instance is unnamed, so
// every instance Loc Decode's errors carry renders "?" for its URI.
func newConfig(opts []Option) config {
	var cfg config
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// WithURI names the instance document data holds, so every instance
// [xsderr.Loc] an error of [Decode] carries identifies the file a reader has
// to open. [Decode] takes a []byte, which carries no name of its own.
func WithURI(uri string) Option {
	return func(c *config) { c.uri = uri }
}
