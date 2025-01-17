// Copyright 2020, Chef.  All rights reserved.
// https://github.com/ysjhlnu/lal
//
// Use of this source code is governed by a MIT-style license
// that can be found in the License file.
//
// Author: Chef (191201771@qq.com)

package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/ysjhlnu/lal/pkg/rtmp"

	"github.com/q191201771/naza/pkg/nazalog"
	"github.com/ysjhlnu/lal/pkg/base"
	"github.com/ysjhlnu/lal/pkg/remux"
	"github.com/ysjhlnu/lal/pkg/rtsp"
)

func main() {
	_ = nazalog.Init(func(option *nazalog.Option) {
		option.AssertBehavior = nazalog.AssertFatal
	})
	defer nazalog.Sync()
	base.LogoutStartInfo()

	inUrl, outUrl, overTcp := parseFlag()

	pushSession := rtmp.NewPushSession(func(option *rtmp.PushSessionOption) {
		option.PushTimeoutMs = 10000
		option.WriteAvTimeoutMs = 10000
	})

	err := pushSession.Push(outUrl)
	nazalog.Assert(nil, err)
	defer pushSession.Dispose()

	remuxer := remux.NewAvPacket2RtmpRemuxer().WithOnRtmpMsg(func(msg base.RtmpMsg) {
		err = pushSession.Write(rtmp.Message2Chunks(msg.Payload, &msg.Header))
		nazalog.Assert(nil, err)
	})
	pullSession := rtsp.NewPullSession(remuxer, func(option *rtsp.PullSessionOption) {
		option.PullTimeoutMs = 10000
		option.OverTcp = overTcp != 0
	})

	err = pullSession.Pull(inUrl)
	nazalog.Assert(nil, err)
	defer pullSession.Dispose()

	go func() {
		for {
			pullSession.UpdateStat(1)
			pullStat := pullSession.GetStat()
			pushSession.UpdateStat(1)
			pushStat := pushSession.GetStat()
			nazalog.Debugf("stat. pull=%+v, push=%+v", pullStat, pushStat)
			time.Sleep(1 * time.Second)
		}
	}()

	select {
	case err = <-pullSession.WaitChan():
		nazalog.Infof("< pullSession.Wait(). err=%+v", err)
		time.Sleep(1 * time.Second)
		return
	case err = <-pushSession.WaitChan():
		nazalog.Infof("< pushSession.Wait(). err=%+v", err)
		time.Sleep(1 * time.Second)
		return
	}
}

func parseFlag() (inUrl string, outUrl string, overTcp int) {
	i := flag.String("i", "", "specify pull rtsp url")
	o := flag.String("o", "", "specify push rtmp url")
	t := flag.Int("t", 0, "specify interleaved mode(rtp/rtcp over tcp)")
	flag.Parse()
	if *i == "" || *o == "" {
		flag.Usage()
		_, _ = fmt.Fprintf(os.Stderr, `Example:
  %s -i rtsp://localhost:5544/live/test110 -o rtmp://localhost:1935/live/test220 -t 0
  %s -i rtsp://localhost:5544/live/test110 -o rtmp://localhost:1935/live/test220 -t 1
`, os.Args[0], os.Args[0])
		base.OsExitAndWaitPressIfWindows(1)
	}
	return *i, *o, *t
}
