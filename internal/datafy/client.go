package datafy

type Client struct {
	config Config
}

func NewDatafyClient(config *Config) *Client {
	return &Client{
		config: *config,
	}
}
