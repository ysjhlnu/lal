// Copyright 2022, Chef.  All rights reserved.
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

	"github.com/q191201771/naza/pkg/nazalog"
	"github.com/ysjhlnu/lal/pkg/aac"
	"github.com/ysjhlnu/lal/pkg/avc"
	"github.com/ysjhlnu/lal/pkg/httpflv"
	"github.com/ysjhlnu/lal/pkg/remux"

	"github.com/ysjhlnu/lal/pkg/base"

	"github.com/q191201771/naza/pkg/bininfo"
	"github.com/ysjhlnu/lal/pkg/logic"
)

// 注意，使用这个demo时，请确保这三个文件存在，文件下载地址 https://github.com/q191201771/lalext/tree/master/avfile
const (
	h264filename = "/tmp/test.h264"
	aacfilename  = "/tmp/test.aac"
	flvfilename  = "/tmp/test.flv"
)

// 文档见 <lalserver二次开发 - pub接入自定义流>
// https://pengrl.com/lal/#/customize_pub
//

// MySession 演示业务方实现 logic.ICustomizeHookSessionContext 接口，从而hook所有输入到lalserver中的流以及流中的数据。
type MySession struct {
	uniqueKey  string
	streamName string
}

func (i *MySession) OnMsg(msg base.RtmpMsg) {
	// 业务方可以在这里对流做处理
	if msg.IsAacSeqHeader() || msg.IsVideoKeySeqHeader() || msg.IsVideoKeyNalu() {
		nazalog.Debugf("%s", msg.DebugString())
	}
}

func (i *MySession) OnStop() {
	nazalog.Debugf("OnStop")
}

func main() {
	defer nazalog.Sync()

	confFilename := parseFlag()
	lals := logic.NewLalServer(func(option *logic.Option) {
		option.ConfFilename = confFilename
	})

	// 在常规lalserver基础上增加这行，用于演示hook lalserver中的流
	lals.WithOnHookSession(func(uniqueKey string, streamName string) logic.ICustomizeHookSessionContext {
		// 有新的流了，创建业务层的对象，用于hook这个流
		return &MySession{
			uniqueKey:  uniqueKey,
			streamName: streamName,
		}
	})

	// 在常规lalserver基础上增加这两个例子，用于演示向lalserver输入自定义流
	go showHowToCustomizePub(lals)
	go showHowToFlvCustomizePub(lals)

	err := lals.RunLoop()
	nazalog.Infof("server manager done. err=%+v", err)
}

func parseFlag() string {
	binInfoFlag := flag.Bool("v", false, "show bin info")
	cf := flag.String("c", "", "specify conf file")
	flag.Parse()

	if *binInfoFlag {
		_, _ = fmt.Fprint(os.Stderr, bininfo.StringifyMultiLine())
		_, _ = fmt.Fprintln(os.Stderr, base.LalFullInfo)
		os.Exit(0)
	}

	return *cf
}

func showHowToFlvCustomizePub(lals logic.ILalServer) {
	const customizePubStreamName = "f110"

	time.Sleep(200 * time.Millisecond)

	tags, err := httpflv.ReadAllTagsFromFlvFile(flvfilename)
	nazalog.Assert(nil, err)

	session, err := lals.AddCustomizePubSession(customizePubStreamName)
	nazalog.Assert(nil, err)

	startRealTime := time.Now()
	startTs := int64(0)
	for _, tag := range tags {
		msg := remux.FlvTag2RtmpMsg(tag)
		diffTs := int64(msg.Header.TimestampAbs) - startTs
		diffReal := time.Since(startRealTime).Milliseconds()
		if diffReal < diffTs {
			time.Sleep(time.Duration(diffTs-diffReal) * time.Millisecond)
		}
		session.FeedRtmpMsg(msg)
	}

	lals.DelCustomizePubSession(session)
}

func showHowToCustomizePub(lals logic.ILalServer) {
	const customizePubStreamName = "c110"

	time.Sleep(200 * time.Millisecond)

	// 从音频和视频各自的ES流文件中读取出所有数据
	// 然后将它们按时间戳排序，合并到一个AvPacket数组中
	audioContent, audioPackets := readAudioPacketsFromFile(aacfilename)
	_, videoPackets := readVideoPacketsFromFile(h264filename)
	packets := mergePackets(audioPackets, videoPackets)

	// 1. 向lalserver中加入自定义的pub session
	session, err := lals.AddCustomizePubSession(customizePubStreamName)
	nazalog.Assert(nil, err)
	// 2. 配置session
	session.WithOption(func(option *base.AvPacketStreamOption) {
		option.VideoFormat = base.AvPacketStreamVideoFormatAnnexb
	})

	asc, err := aac.MakeAscWithAdtsHeader(audioContent[:aac.AdtsHeaderLength])
	nazalog.Assert(nil, err)
	// 3. 填入aac的audio specific config信息
	session.FeedAudioSpecificConfig(asc)

	// 4. 按时间戳间隔匀速发送音频和视频
	startRealTime := time.Now()
	startTs := int64(0)
	for i := range packets {
		diffTs := packets[i].Timestamp - startTs
		diffReal := time.Now().Sub(startRealTime).Milliseconds()
		//nazalog.Debugf("%d: %s, %d, %d", i, packets[i].DebugString(), diffTs, diffReal)
		if diffReal < diffTs {
			time.Sleep(time.Duration(diffTs-diffReal) * time.Millisecond)
		}
		session.FeedAvPacket(packets[i])
	}

	// 5. 所有数据发送关闭后，将pub session从lal server移除
	lals.DelCustomizePubSession(session)
}

// readAudioPacketsFromFile 从aac es流文件读取所有音频包
func readAudioPacketsFromFile(filename string) (audioContent []byte, audioPackets []base.AvPacket) {
	var err error
	audioContent, err = os.ReadFile(filename)
	nazalog.Assert(nil, err)

	pos := 0
	timestamp := float32(0)
	for {
		ctx, err := aac.NewAdtsHeaderContext(audioContent[pos : pos+aac.AdtsHeaderLength])
		nazalog.Assert(nil, err)

		packet := base.AvPacket{
			PayloadType: base.AvPacketPtAac,
			Timestamp:   int64(timestamp),
			Payload:     audioContent[pos+aac.AdtsHeaderLength : pos+int(ctx.AdtsLength)],
		}

		audioPackets = append(audioPackets, packet)

		timestamp += float32(48000*4*2) / float32(8192*2) // (frequence * bytePerSample * channel) / (packetSize * channel)

		pos += int(ctx.AdtsLength)
		if pos == len(audioContent) {
			break
		}
	}

	return
}

// readVideoPacketsFromFile 从h264 es流文件读取所有视频包
func readVideoPacketsFromFile(filename string) (videoContent []byte, videoPackets []base.AvPacket) {
	var err error
	videoContent, err = os.ReadFile(filename)
	nazalog.Assert(nil, err)

	timestamp := float32(0)
	err = avc.IterateNaluAnnexb(videoContent, func(nal []byte) {
		// 将nal数据转换为lalserver要求的格式输入
		packet := base.AvPacket{
			PayloadType: base.AvPacketPtAvc,
			Timestamp:   int64(timestamp),
			Payload:     append(avc.NaluStartCode4, nal...),
		}

		videoPackets = append(videoPackets, packet)

		t := avc.ParseNaluType(nal[0])
		if t == avc.NaluTypeSps || t == avc.NaluTypePps || t == avc.NaluTypeSei {
			// noop
		} else {
			timestamp += float32(1000) / float32(15) // 1秒 / fps
		}
	})
	nazalog.Assert(nil, err)

	return
}

// mergePackets 将音频队列和视频队列按时间戳有序合并为一个队列
func mergePackets(audioPackets, videoPackets []base.AvPacket) (packets []base.AvPacket) {
	var i, j int
	for {
		// audio数组为空，将video的剩余数据取出，然后merge结束
		if i == len(audioPackets) {
			packets = append(packets, videoPackets[j:]...)
			break
		}

		//
		if j == len(videoPackets) {
			packets = append(packets, audioPackets[i:]...)
			break
		}

		// 音频和视频都有数据，取时间戳小的
		if audioPackets[i].Timestamp < videoPackets[j].Timestamp {
			packets = append(packets, audioPackets[i])
			i++
		} else {
			packets = append(packets, videoPackets[j])
			j++
		}
	}

	return
}
