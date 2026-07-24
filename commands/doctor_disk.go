package commands

import "github.com/agent-dance/luban/i18n"

func diskSpaceResult(freeBytes uint64, lang i18n.Language) checkResult {
	freeGB := float64(freeBytes) / (1 << 30)
	r := checkResult{ok: freeGB >= 1.0}
	if r.ok {
		r.message = i18n.Format(lang, i18n.KeyDoctorDiskFree, freeGB)
	} else {
		r.message = i18n.Format(lang, i18n.KeyDoctorDiskLow, freeGB)
	}
	return r
}
