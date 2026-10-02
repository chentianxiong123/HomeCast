package bilibili

import (
	"net/url"
	"regexp"
)

// SearchItem 搜索结果条目（对齐 Python SearchItem）
type SearchItem struct {
	ID        int    `json:"id"`
	BVID      string `json:"bvid"`
	Title     string `json:"title"`
	Desc      string `json:"description"`
	Duration  string `json:"duration"`
	Pic       string `json:"pic"`
	Author    string `json:"author"`
	MID       int    `json:"mid"`
	Play      int    `json:"play"`
	VideoReview int  `json:"video_review"`
}

// SearchResult 搜索返回
type SearchResult struct {
	NumResults int          `json:"numResults"`
	Result     []SearchItem `json:"result"`
}

var htmlTagRe = regexp.MustCompile(`<[^>]+>`)

// CleanHTML 剥掉搜索标题里的 <em class="keyword"> 高亮标签
func CleanHTML(text string) string {
	return htmlTagRe.ReplaceAllString(text, "")
}

// Search 全站搜索视频（对齐 /x/web-interface/search/type）
func (c *Client) Search(keyword string, page, pageSize int) (*SearchResult, error) {
	params := url.Values{}
	params.Set("keyword", keyword)
	params.Set("search_type", "video")
	params.Set("page", itoa(page))
	params.Set("pagesize", itoa(pageSize))
	data, err := c.GetJSON("/x/web-interface/search/type", params)
	if err != nil {
		return nil, err
	}
	var res SearchResult
	if err := decodeData(data, &res); err != nil {
		return nil, err
	}
	for i := range res.Result {
		res.Result[i].Title = CleanHTML(res.Result[i].Title)
	}
	return &res, nil
}

// SearchSuggest 搜索建议（s.search.bilibili.com）
func (c *Client) SearchSuggest(keyword string) ([]string, error) {
	params := url.Values{}
	params.Set("term", keyword)
	data, err := c.GetJSON("https://s.search.bilibili.com/main/suggest", params)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, v := range data {
		if s, ok := v.(string); ok {
			out = append(out, s)
		}
	}
	return out, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
