package app

import (
	"net/http"
	"strconv"
	"strings"
)

type (
	Title struct {
		// Names      interface{}
		// Updated    uint64
		// LastChange uint64 `json:"last_change"`

		Id     uint16
		Code   string
		Player *Player
	}
	Player struct {
		// AlternativePlayer string `json:"alternative_player"`
		// Series            interface{}

		Host     string
		Playlist map[string]*PlayerPlaylist
	}
	PlayerPlaylist struct {
		// CreatedTimestamp uint64 `json:"created_timestamp"`
		// Preview          string
		// Skips            interface{}

		Serie uint16
		Hls   *PlaylistHls
	}
	PlaylistHls struct {
		Fhd, Hd, Sd string
	}
)

type (
	titleQuality uint8
	TitleSerie   struct {
		Title, Serie  uint16
		QualityHashes map[titleQuality]string
	}
)

const (
	titleQualityNone titleQuality = iota
	titleQualitySD
	titleQualityHD
	titleQualityFHD
)

// redis schema
// https://cache.libria.fun/videos/media/ts/9277/13/720/3ae5aa5839690b8d9ea9fcef9b720fb4_00028.ts
// https://cache.libria.fun/videos/media/ts/9222/11/1080/97d3bb428727bc25fa110bc51826a366.m3u8
// KEY - VALUE
// ID:SERIE - FHD:HD:SD (links)
// ID:SERIE:{FHD,HD,SD} - json{[TS LINKS]}

func (*App) getTitlePlayerLinks() (e error) {
	return
}

func (*App) getTitleCacheStatus() (ok bool, e error) {
	return
}

func (m *App) getTitleSerieFromCache(titleId, serieId uint16) (*TitleSerie, bool) {
	serie := m.cache.PullSerie(titleId, serieId)
	return serie, serie != nil
}

// TODO - remove strconv
func (m *App) getTitleFromApi(titleId uint16) (title *Title, e error) {
	e = gAniApi.getApiResponse(http.MethodGet, apiMethodGetTitle,
		[]string{"id", strconv.Itoa(int(titleId))}).parseApiResponse(&title)
	return
}

func (*App) cacheTitleFromRespose(title *Title) {
	for _, serie := range title.Player.Playlist {
		if serie == nil {
			log.Warn().Msg("there is an empty serie found in the api response's playlist")
			continue
		}

		if serie.Hls == nil {
			log.Warn().Msg("there is an empty serie.HLS found in the api response's playlist")
			continue
		}

		tserie, ok := &TitleSerie{}, false

		// parse

		// !!! XXX !!! HERE MUST BE THE HASHED BUT NOT URLS!!!!
		if tserie.QualityHashes[titleQualitySD], ok = serie.getQuality(titleQualitySD); !ok {
			log.Warn().Uint16("tid", tserie.Title).Uint16("sed", tserie.Serie).
				Msg("there is no SD quality for parsed title")
		}
		if tserie.QualityHashes[titleQualityHD], ok = serie.getQuality(titleQualityHD); !ok {
			log.Warn().Uint16("tid", tserie.Title).Uint16("sed", tserie.Serie).
				Msg("there is no HD quality for parsed title")
		}
		if tserie.QualityHashes[titleQualityFHD], ok = serie.getQuality(titleQualityFHD); !ok {
			log.Warn().Uint16("tid", tserie.Title).Uint16("sed", tserie.Serie).
				Msg("there is no FHD quality for parsed title")
		}

		// !!!
		// !!!
		// !!!
		// !!!
		// !!!
		// !!!
		// !!!
		// !!!
		// !!!
		// !!!
		// !!!

	}
}

func (m *PlayerPlaylist) getQuality(quality titleQuality) (string, bool) {
	switch quality {
	case titleQualitySD:
		return m.Hls.Sd, m.Hls.Sd != ""
	case titleQualityHD:
		return m.Hls.Hd, m.Hls.Hd != ""
	case titleQualityFHD:
		return m.Hls.Fhd, m.Hls.Fhd != ""
	default:
		return "", false
	}
}

type TitleSerieRequest struct {
	titileId, serieId uint16
	quality           titleQuality
	hash              string

	raw []string
}

const (
	tsrRawTitleId = uint8(iota) + 3 // 3 is a skipping of "/videos/media/ts" parts
	tsrRawTitleSerie
	tsrRawTitleQuality
	tsrRawFilename
)

func NewTitleSerieRequest(uri string) *TitleSerieRequest {
	return &TitleSerieRequest{
		raw: strings.Split(uri, "/"),
	}
}

func (m *TitleSerieRequest) getTitleId() (_ uint16, e error) {
	var tid int
	if tid, e = strconv.Atoi(m.raw[tsrRawTitleId]); e != nil {
		return
	}

	m.titileId = uint16(tid)
	return m.titileId, e
}

func (m *TitleSerieRequest) getSerieId() (_ uint16, e error) {
	var tid int
	if tid, e = strconv.Atoi(m.raw[tsrRawTitleSerie]); e != nil {
		return
	}

	m.titileId = uint16(tid)
	return m.titileId, e
}

func (m *TitleSerieRequest) getTitleQuality() titleQuality {
	switch m.raw[tsrRawTitleQuality] {
	case "480":
		m.quality = titleQualitySD
		return m.quality
	case "720":
		m.quality = titleQualityHD
		return m.quality
	case "1080":
		m.quality = titleQualityFHD
		return m.quality
	default:
		return titleQualityNone
	}
}

func (m *TitleSerieRequest) getTitleHash() (_ string, ok bool) {
	switch m.raw[tsrRawFilename][len(m.raw[tsrRawFilename])-1:] {
	case "s": // .ts
		if m.hash, _, ok = strings.Cut(m.raw[tsrRawFilename], ".ts"); !ok {
			return "", ok
		}
	case "8": // .m3u8
		if m.hash, _, ok = strings.Cut(m.raw[tsrRawFilename], ".m3u8"); !ok {
			return "", ok
		}
	default:
		return "", false
	}

	return m.hash, ok
}

func (*App) getTitleHash(titleId, serieId uint16, quality titleQuality) string {
	return ""
}
