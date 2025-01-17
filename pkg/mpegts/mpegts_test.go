// Copyright 2020, Chef.  All rights reserved.
// https://github.com/ysjhlnu/lal
//
// Use of this source code is governed by a MIT-style license
// that can be found in the License file.
//
// Author: Chef (191201771@qq.com)

package mpegts_test

import (
	"testing"

	"github.com/ysjhlnu/lal/pkg/innertest"

	"github.com/ysjhlnu/lal/pkg/mpegts"
)

func TestMpegts(t *testing.T) {
	innertest.Entry(t)
}

func TestParseFixedTsPacket(t *testing.T) {
	h := mpegts.ParseTsPacketHeader(mpegts.FixedFragmentHeader)
	mpegts.Log.Debugf("%+v", h)
	pat := mpegts.ParsePat(mpegts.FixedFragmentHeader[5:])
	mpegts.Log.Debugf("%+v", pat)

	h = mpegts.ParseTsPacketHeader(mpegts.FixedFragmentHeaderHevc[188:])
	mpegts.Log.Debugf("%+v", h)
	pmt := mpegts.ParsePmt(mpegts.FixedFragmentHeader[188+5:])
	mpegts.Log.Debugf("%+v", pmt)
}
