package app

import (
	"errors"
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

const (
	tsrRawTitleId = uint8(iota) + 4 // 4 is a skipping of "/videos/media/ts" parts
	tsrRawTitleSerie
	tsrRawTitleQuality
	tsrRawFilename
)

// // TODO - remove strconv
func getHashFromUriPath(upath string) (hash string, ok bool) {
	switch upath[len(upath)-1:] {
	case "s": // .ts
		if hash, _, ok = strings.Cut(upath, ".ts"); !ok {
			return "", ok
		}
	case "8": // .u8
		if hash, _, ok = strings.Cut(upath, ".u8"); !ok {
			return "", ok
		}
	default:
		return "", false
	}

	return hash, ok
}

func (m *App) getTitleSerieFromCache(tsr *TitleSerieRequest) (*TitleSerie, bool) {
	serie, e := m.cache.PullSerie(tsr.titileId, tsr.serieId)
	if e != nil {
		return nil, false
	}

	return serie, serie != nil
}

// TODO - remove strconv
func (m *App) getTitleSeriesFromApi(titleId uint16) (_ []*TitleSerie, e error) {
	var title *Title
	e = gAniApi.getApiResponse(http.MethodGet, apiMethodGetTitle,
		[]string{"id", strconv.Itoa(int(titleId))}).parseApiResponse(&title)

	return m.validateTitleFromApiResponse(title), e
}

func (m *App) validateTitleFromApiResponse(title *Title) (tss []*TitleSerie) {
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

		tserie.Title = title.Id
		tserie.Serie = serie.Serie

		if tserie.QualityHashes[titleQualitySD], ok = getHashFromUriPath(strings.Split(serie.Hls.Sd, "/")[tsrRawFilename]); !ok {
			log.Warn().Uint16("tid", tserie.Title).Uint16("sed", tserie.Serie).Msg("there is no SD quality for parsed title")
		}

		if tserie.QualityHashes[titleQualityHD], ok = getHashFromUriPath(strings.Split(serie.Hls.Hd, "/")[tsrRawFilename]); !ok {
			log.Warn().Uint16("tid", tserie.Title).Uint16("sed", tserie.Serie).Msg("there is no HD quality for parsed title")
		}

		if tserie.QualityHashes[titleQualityFHD], ok = getHashFromUriPath(strings.Split(serie.Hls.Fhd, "/")[tsrRawFilename]); !ok {
			log.Warn().Uint16("tid", tserie.Title).Uint16("sed", tserie.Serie).Msg("there is no FHD quality for parsed title")
		}

		tss = append(tss, tserie)
	}

	return
}

func (m *App) doTitleSerieRequest(tsr *TitleSerieRequest) (ts *TitleSerie, e error) {
	// get from cache
	// get from api -> cache

	var ok bool
	log.Info().Uint16("", tsr.titileId).Uint16("", tsr.serieId).Msg("")
	if ts, ok = m.getTitleSerieFromCache(tsr); ok {
		return
	}

	var tss []*TitleSerie
	log.Info().Uint16("", tsr.titileId).Uint16("", tsr.serieId).Msg("")
	if tss, e = m.getTitleSeriesFromApi(tsr.titileId); e != nil {
		return
	}

	if len(tss) == 0 {
		return nil, errors.New("")
	}

	log.Info().Uint16("", tsr.titileId).Uint16("", tsr.serieId).Msg("")
	for _, t := range tss {
		if t.Serie == tsr.serieId {
			ts = t
		}

		if e = m.cache.PushSerie(t); e != nil {
			log.Warn().Uint16("", tsr.titileId).Uint16("", tsr.serieId).Msg("")
			continue
		}
	}

	if ts == nil {
		return nil, errors.New("")
	}

	return
}

type TitleSerieRequest struct {
	titileId, serieId uint16
	quality           titleQuality
	hash              string

	raw []string
}

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
	return getHashFromUriPath(m.raw[tsrRawFilename])
}
