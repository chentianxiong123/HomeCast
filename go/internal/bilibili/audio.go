package bilibili

import "net/url"

// B站真实 qn 码（桌面项目已验证，Python 版同款）
const (
	Audio64K  = 30216 // 64k
	Audio128K = 30232 // 128k
	Audio192K = 30280 // 192k（免费最高）
	AudioFLAC = 30250 // FLAC
)

// QualityPriority 音质优先级：192k 优先
var QualityPriority = []int{Audio192K, Audio128K, Audio64K, AudioFLAC}

// PreferredQuality 默认音质（桌面 PREFERRED_QN）
const PreferredQuality = Audio192K

// DashAudioItem DASH 音频条目
type DashAudioItem struct {
	ID        int      `json:"id"`
	BaseURL   string   `json:"baseUrl"`
	BackupURL []string `json:"backupUrl"`
	Bandwidth int      `json:"bandwidth"`
	MimeType  string   `json:"mimeType"`
	Codecs    string   `json:"codecs"`
	Size      int      `json:"size"`
}

// DashInfo DASH 信息
type DashInfo struct {
	Duration int             `json:"duration"`
	Video    []DashVideoItem `json:"video"`
	Audio    []DashAudioItem `json:"audio"`
}

// DashVideoItem DASH 视频条目（投屏用）
type DashVideoItem struct {
	ID        int      `json:"id"`
	BaseURL   string   `json:"baseUrl"`
	BackupURL []string `json:"backupUrl"`
	Width     int      `json:"width"`
	Height    int      `json:"height"`
	Bandwidth int      `json:"bandwidth"`
	MimeType  string   `json:"mimeType"`
	Codecs    string   `json:"codecs"`
	Size      int      `json:"size"`
}

// AudioStreamInfo playurl 返回
type AudioStreamInfo struct {
	Quality  int      `json:"quality"`
	Format   string   `json:"format"`
	Dash     *DashInfo `json:"dash"`
	Durl     []struct {
		URL  string `json:"url"`
		Size int    `json:"size"`
	} `json:"durl"`
}

// AudioStreamResult 最终取流结果
type AudioStreamResult struct {
	URL      string
	Quality  int
	Size     int
	MimeType string
	Codecs   string
}

// GetAudioStream 纯 DASH 音频模式取流。
// 关键：fnval=16；绝不加 platform=html5/mobisel/highbit（会强制 durl 视频模式）。
func (c *Client) GetAudioStream(bvid string, cid int, quality int) (*AudioStreamInfo, error) {
	params := url.Values{}
	params.Set("bvid", bvid)
	params.Set("cid", itoa(cid))
	params.Set("qn", itoa(quality))
	params.Set("fnval", "16")
	params.Set("fourk", "1")
	data, err := c.GetJSON("/x/player/playurl", params)
	if err != nil {
		return nil, err
	}
	var info AudioStreamInfo
	if err := decodeData(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}

// GetBestAudioURL 取音频 URL：DASH 纯音频优先（桌面项目主路径）
func (c *Client) GetBestAudioURL(bvid string, cid int, preferQuality int) (*AudioStreamResult, error) {
	stream, err := c.GetAudioStream(bvid, cid, preferQuality)
	if err != nil {
		return nil, err
	}
	if stream.Dash != nil && len(stream.Dash.Audio) > 0 {
		byID := map[int]DashAudioItem{}
		for _, a := range stream.Dash.Audio {
			byID[a.ID] = a
		}
		order := []int{preferQuality}
		for _, q := range QualityPriority {
			if q != preferQuality {
				order = append(order, q)
			}
		}
		for _, qn := range order {
			if a, ok := byID[qn]; ok {
				u := a.BaseURL
				if u == "" && len(a.BackupURL) > 0 {
					u = a.BackupURL[0]
				}
				if u != "" {
					return &AudioStreamResult{URL: u, Quality: qn, Size: a.Size, MimeType: a.MimeType, Codecs: a.Codecs}, nil
				}
			}
		}
		a := stream.Dash.Audio[0]
		u := a.BaseURL
		if u == "" && len(a.BackupURL) > 0 {
			u = a.BackupURL[0]
		}
		return &AudioStreamResult{URL: u, Quality: a.ID, Size: a.Size, MimeType: a.MimeType, Codecs: a.Codecs}, nil
	}
	if len(stream.Durl) > 0 {
		return &AudioStreamResult{URL: stream.Durl[0].URL, Quality: preferQuality, Size: stream.Durl[0].Size}, nil
	}
	return nil, &BilibiliAPIError{Code: -1, Message: "no audio stream available"}
}

// GetBestVideoURL 取 DASH 视频流 URL（投屏用；带 referer 代理给电视）
func (c *Client) GetBestVideoURL(bvid string, cid int) (*DashVideoItem, error) {
	stream, err := c.GetAudioStream(bvid, cid, AudioFLAC)
	if err != nil {
		return nil, err
	}
	if stream.Dash != nil && len(stream.Dash.Video) > 0 {
		// 按码率降序取最高（一般第一个即最高，这里稳妥排序）
		best := stream.Dash.Video[0]
		for _, v := range stream.Dash.Video[1:] {
			if v.Bandwidth > best.Bandwidth {
				best = v
			}
		}
		if best.BaseURL == "" && len(best.BackupURL) > 0 {
			best.BaseURL = best.BackupURL[0]
		}
		return &best, nil
	}
	return nil, &BilibiliAPIError{Code: -1, Message: "no video stream available"}
}

// VideoInfo 视频信息（拿 cid 用）
type VideoInfo struct {
	BVID     string `json:"bvid"`
	AID      int    `json:"aid"`
	CID      int    `json:"cid"`
	Title    string `json:"title"`
	Desc     string `json:"desc"`
	Pic      string `json:"pic"`
	Duration int    `json:"duration"`
	Owner    struct {
		MID  int    `json:"mid"`
		Name string `json:"name"`
		Face string `json:"face"`
	} `json:"owner"`
}

// GetVideoInfo /x/web-interface/view
func (c *Client) GetVideoInfo(bvid string) (*VideoInfo, error) {
	params := url.Values{}
	params.Set("bvid", bvid)
	data, err := c.GetJSON("/x/web-interface/view", params)
	if err != nil {
		return nil, err
	}
	var info VideoInfo
	if err := decodeData(data, &info); err != nil {
		return nil, err
	}
	return &info, nil
}
