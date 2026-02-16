package search

type SearchResult struct {
	Title string `json:"title"`
	URL   string `json:"url"`
	Body  string `json:"body"`
}

type SearchOptions struct {
	MaxResults int    `json:"max_results"`
	Region     string `json:"region"`
	SafeSearch string `json:"safesearch"`
	TimeLimit  string `json:"timelimit"`
}

func DefaultOptions() SearchOptions {
	return SearchOptions{
		MaxResults: 10,
		Region:     "us-en",
		SafeSearch: "moderate",
	}
}
