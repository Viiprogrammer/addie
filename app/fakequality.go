package app

import "strings"

func (m *App) getUriWithFakeQuality(tsr *TitleSerieRequest, uri string, quality titleQuality) string {
	gLog.Debug().Msg("format check")
	if tsr.isOldFormat() && !tsr.isM3U8() {
		gLog.Info().Str("old", "/"+tsr.getTitleQualityString()+"/").Str("new", "/"+quality.string()+"/").Str("uri", uri).Msg("format is old")
		return strings.ReplaceAll(uri, "/"+tsr.getTitleQualityString()+"/", "/"+quality.string()+"/")
	}

	gLog.Debug().Msg("trying to complete tsr")
	title, e := m.doTitleSerieRequest(tsr)
	if e != nil {
		gLog.Error().Err(e).Msg("could not rewrite quality for the request")
		return uri
	}

	gLog.Debug().Msg("trying to get hash")
	hash, ok := tsr.getTitleHash()
	if !ok {
		return uri
	}

	gLog.Debug().Str("old_hash", hash).Str("new_hash", title.QualityHashes[quality]).Str("uri", uri).Msg("")
	return strings.ReplaceAll(
		strings.ReplaceAll(uri, "/"+tsr.getTitleQualityString()+"/", "/"+quality.string()+"/"),
		hash, title.QualityHashes[quality],
	)
}
