// Copyright 2020, Chef.  All rights reserved.
// https://github.com/q191201771/lal
//
// Use of this source code is governed by a MIT-style license
// that can be found in the License file.
//
// Author: Chef (191201771@qq.com)

package base

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
)

// 见单元测试

// TODO chef: 考虑部分内容移入naza中

const (
	DefaultRtmpPort  = 1935
	DefaultHttpPort  = 80
	DefaultHttpsPort = 443
	DefaultRtspPort  = 554
	DefaultRtmpsPort = 443
	DefaultRtspsPort = 322
)

type UrlPathContext struct {
	PathWithRawQuery    string
	Path                string
	PathWithoutLastItem string // 注意，没有前面的'/'，也没有后面的'/'
	LastItemOfPath      string // 注意，没有前面的'/'
	RawQuery            string
}

type UrlContext struct {
	Url string

	Scheme       string
	Username     string
	Password     string
	StdHost      string // host or host:port
	HostWithPort string // 当原始url中不包含port时，填充scheme对应的默认port
	Host         string // 不包含port
	Port         int    // 当原始url中不包含port时，填充scheme对应的默认port

	//UrlPathContext
	PathWithRawQuery    string // 注意，有前面的'/'
	Path                string // 注意，有前面的'/'
	PathWithoutLastItem string // 注意，没有前面的'/'，也没有后面的'/'
	LastItemOfPath      string // 注意，没有前面的'/'
	RawQuery            string // 参数，注意，没有前面的'?'

	RawUrlWithoutUserInfo string

	filenameWithoutType string
	fileType            string
}

func (u *UrlContext) GetFilenameWithoutType() string {
	u.calcFilenameAndTypeIfNeeded()
	return u.filenameWithoutType
}

func (u *UrlContext) GetFileType() string {
	u.calcFilenameAndTypeIfNeeded()
	return u.fileType
}

func (u *UrlContext) calcFilenameAndTypeIfNeeded() {
	if len(u.filenameWithoutType) == 0 || len(u.fileType) == 0 {
		ss := strings.Split(u.LastItemOfPath, ".")
		u.filenameWithoutType = ss[0]
		if len(ss) > 1 {
			u.fileType = ss[1]
		}
	}
}

// ---------------------------------------------------------------------------------------------------------------------

// ParseUrl
//
// @param defaultPort:
// 注意，如果rawUrl中显示指定了端口，则该参数不生效。
// 注意，如果设置为-1，内部依然会对常见协议(http, https, rtmp, rtsp)设置官方默认端口。
func ParseUrl(rawUrl string, defaultPort int) (ctx UrlContext, err error) {
	ctx.Url = rawUrl

	stdUrl, err := url.Parse(rawUrl)
	if err != nil {
		return ctx, err
	}
	if stdUrl.Scheme == "" {
		return ctx, fmt.Errorf("%w. url=%s", ErrInvalidUrl, rawUrl)
	}
	// 如果不存在，则设置默认的
	if defaultPort == -1 {
		// TODO(chef): 测试大小写的情况
		switch stdUrl.Scheme {
		case "http":
			defaultPort = DefaultHttpPort
		case "https":
			defaultPort = DefaultHttpsPort
		case "rtmp":
			defaultPort = DefaultRtmpPort
		case "rtsp":
			defaultPort = DefaultRtspPort
		case "rtmps":
			defaultPort = DefaultRtmpsPort
		case "rtsps":
			defaultPort = DefaultRtspsPort
		}
	}

	ctx.Scheme = stdUrl.Scheme
	ctx.StdHost = stdUrl.Host
	ctx.Username = stdUrl.User.Username()
	ctx.Password, _ = stdUrl.User.Password()

	h, p, err := net.SplitHostPort(stdUrl.Host)
	if err != nil {
		// url中端口不存在

		ctx.Host = stdUrl.Host
		if defaultPort == -1 {
			ctx.HostWithPort = stdUrl.Host
		} else {
			ctx.HostWithPort = net.JoinHostPort(stdUrl.Host, fmt.Sprintf("%d", defaultPort))
			ctx.Port = defaultPort
		}
	} else {
		// 端口存在

		ctx.Port, err = strconv.Atoi(p)
		if err != nil {
			return ctx, err
		}
		ctx.Host = h
		ctx.HostWithPort = stdUrl.Host
	}

	pathCtx, err := parseUrlPath(stdUrl)
	if err != nil {
		return ctx, err
	}
	ctx.PathWithRawQuery = pathCtx.PathWithRawQuery
	ctx.Path = pathCtx.Path
	ctx.PathWithoutLastItem = pathCtx.PathWithoutLastItem
	ctx.LastItemOfPath = pathCtx.LastItemOfPath
	ctx.RawQuery = pathCtx.RawQuery

	ctx.RawUrlWithoutUserInfo = fmt.Sprintf("%s://%s%s", ctx.Scheme, ctx.StdHost, ctx.PathWithRawQuery)
	return ctx, nil
}

// ---------------------------------------------------------------------------------------------------------------------

func ParseRtmpUrl(rawUrl string) (ctx UrlContext, err error) {
	ctx, err = ParseUrl(rawUrl, -1)
	if err != nil {
		return
	}
	if ctx.Scheme != "rtmp" && ctx.Scheme != "rtmps" || ctx.Host == "" || ctx.Path == "" {
		return ctx, fmt.Errorf("%w. url=%s", ErrInvalidUrl, rawUrl)
	}

	// 处理特殊case，具体见 testParseRtmpUrlCase1
	// 注意，使用ffmpeg推流时，会把`rtmp://127.0.0.1/test110`中的test110作为appName(streamName则为空)
	// 这种其实已不算十分合法的rtmp url了
	// 我们这里也处理一下，和ffmpeg保持一致
	if ctx.PathWithoutLastItem == "" && ctx.LastItemOfPath != "" {
		tmp := ctx.PathWithoutLastItem
		ctx.PathWithoutLastItem = ctx.LastItemOfPath
		ctx.LastItemOfPath = tmp
	}

	// 处理特殊case, 具体见 testParseRtmpUrlCase2
	//
	// PathWithRawQuery:/vyun?vhost=thirdVhost?token=88F4/lss_7
	//
	// Path:/vyun-----------------------------------------------> /vyun?vhost=thirdVhost?token=88F4/lss_7
	// PathWithoutLastItem:vyun---------------------------------> vyun?vhost=thirdVhost?token=88F4
	// LastItemOfPath:------------------------------------------> lss_7
	// RawQuery:vhost=thirdVhost?token=88F4/lss_7---------------> 空
	//
	if strings.Count(ctx.PathWithRawQuery, "?") > 1 {
		index := strings.LastIndexByte(ctx.PathWithRawQuery, '/')
		ctx.Path = ctx.PathWithRawQuery
		ctx.PathWithoutLastItem = ctx.PathWithRawQuery[1:index]
		ctx.LastItemOfPath = ctx.PathWithRawQuery[index+1:]
		ctx.RawQuery = ""
	}

	return
}

func ParseRtmpUrl2(rawUrl string) (ctx UrlContext, err error) {
	base := rawUrl
	ctx.Url = rawUrl
	ctx.RawUrlWithoutUserInfo = rawUrl
	p := strings.Index(base, "://")
	if p == -1 {
		return ctx, errors.New("RTMP URL: No :// in url")
	}
	//log.Printf("p len: %d\n", p)

	rawSchema := strings.ToLower(base[:p])

	switch rawSchema {
	case "rtmp":
		if len(rawSchema) != 4 {
			return ctx, errors.New("schema error")
		}
	case "rtmpt":
		if len(rawSchema) != 5 {
			return ctx, errors.New("schema error")
		}
	case "rtmps":
		if len(rawSchema) != 5 {
			return ctx, errors.New("schema error")
		}
	case "rtmpe":
		if len(rawSchema) != 5 {
			return ctx, errors.New("schema error")
		}
	case "rtmfp":
		if len(rawSchema) != 5 {
			return ctx, errors.New("schema error")
		}
	case "rtmpte":
		if len(rawSchema) != 6 {
			return ctx, errors.New("schema error")
		}
	case "rtmpts":
		if len(rawSchema) != 6 {
			return ctx, errors.New("schema error")
		}
	default:
		return ctx, errors.New("unknown protocol")
	}
	ctx.Scheme = rawSchema
	//log.Printf("schema: %s\n", rawSchema)

	// 获取主机名称
	//跳过“://”
	base = base[p+3:]
	p = 0 // 每次更新base都需要重置p为0
	//log.Println(base)
	if len(base) == 0 {
		return ctx, errors.New("no hostname in URL")
	}

	/* 检查一下主机名 */

	end := 0

	col := strings.Index(base, ":")
	ques := strings.Index(base, "?")

	slash := strings.IndexByte(base, '/')
	if slash == -1 {
		return ctx, errors.New("rtmp url error")
	}

	//log.Printf("len: %d, col: %d, ques: %d, slash: %d\n", end, col, ques, slash)
	{
		var hostLen int
		if slash != -1 {
			hostLen = slash - p
		} else {
			hostLen = end - p // 取绝对值
		}

		if col != -1 && col-p < hostLen {
			hostLen = col - p
		}

		if hostLen < 256 {
			ctx.Host = base[:hostLen]
			//log.Printf("host len: %d\n", hostLen)
			//log.Printf("host: %s\n", url.Host)
		} else {
			log.Println("Hostname exceeds 255 characters!")
		}
		base = base[hostLen:]

		p = 0
	}

	//log.Printf("base: %s\n", base)

	if strings.HasPrefix(base, ":") {
		portLen := slash - col - 1
		rawPort := base[1 : portLen+1]
		ctx.Port, err = strconv.Atoi(rawPort)
		if err != nil {
			return ctx, err
		}
		if ctx.Port > 65535 {
			return ctx, errors.New("invalid port number")
		}
		base = base[portLen+1+1:] // 多加一个1是跳过第一个/
		//log.Println("port len: ", portLen)
		//log.Println("raw port: ", rawPort)
	} else {
		base = base[1:]
		switch ctx.Scheme {
		case "http":
			ctx.Port = DefaultHttpPort
		case "https":
			ctx.Port = DefaultHttpsPort
		case "rtmp":
			ctx.Port = DefaultRtmpPort
		case "rtsp":
			ctx.Port = DefaultRtspPort
		case "rtmps":
			ctx.Port = DefaultRtmpsPort
		case "rtsps":
			ctx.Port = DefaultRtspsPort
		}
	}

	if slash == -1 {
		return ctx, errors.New("no application or playpath in URL")
	}
	ctx.HostWithPort = fmt.Sprintf("%s:%d", ctx.Host, ctx.Port)
	ctx.StdHost = fmt.Sprintf("%s:%d", ctx.Host, ctx.Port)
	ctx.PathWithRawQuery = path.Join("/", base)
	ctx.Path = path.Join("/", base)
	{
		//log.Printf("base: %s\n", base)
		var (
			appLen, appNameLen int
			slash2, slash3     int
		)

		// 指向第二个斜杠
		slash2 = strings.Index(base, "/")
		if slash2 != -1 {
			//指向第三个斜杠
			//temp := base[1:]
			//log.Printf("base: %s\n", base)
			slash3 = strings.Index(base, "/")
			//log.Println("slash3 index: ", slash3)
			//log.Println("slash3: ", base[slash3:])
		}
		//log.Println("slash2 index: ", slash2)
		//log.Println("slash2: ", base[slash2:])
		p = len(base)
		appLen = p - end
		//log.Println("app len: ", appLen)

		if ques != -1 && strings.Contains(base, "slist=") {
			/* whatever it is, the '?' and slist= means we need to use everything as app and parse plapath from slist= */
			appNameLen = ques - p
			//log.Println("app name len: ", appNameLen)
		} else if strings.HasPrefix(base, "ondemand/") {
			/* app = ondemand/foobar, only pass app=ondemand */
			appLen = 8
			appNameLen = 8
		} else { /* app!=ondemand, so app is app[/appinstance] */
			if slash3 != -1 {
				appNameLen = slash3
				//log.Println("app name len: ", appNameLen)
			} else if slash2 != -1 {

				appNameLen = slash2
				//log.Println("app name len: ", appNameLen)
			}
			appLen = appNameLen
		}
		ctx.PathWithoutLastItem = base[:appLen]
		//log.Printf("app len: %d, %s\n", appLen, url.App)

		base = base[appLen:]
		//log.Println("base: ", base)

		if len(base) > 0 {
			ctx.LastItemOfPath = base[1:]
			//log.Println("play path: ", url.PlayPath)
		}
	}
	return ctx, nil
}

func ParseRtspUrl(rawUrl string) (ctx UrlContext, err error) {
	ctx, err = ParseUrl(rawUrl, -1)
	if err != nil {
		return
	}
	// 注意，存在一种情况，使用rtsp pull session，直接拉取没有url path的流，所以不检查ctx.Path
	if (ctx.Scheme != "rtsp" && ctx.Scheme != "rtsps") || ctx.Host == "" {
		return ctx, fmt.Errorf("%w. url=%s", ErrInvalidUrl, rawUrl)
	}

	return
}

func ParseHttpflvUrl(rawUrl string) (ctx UrlContext, err error) {
	return parseHttpUrl(rawUrl, ".flv")
}

// ---------------------------------------------------------------------------------------------------------------------

// ParseHttpRequest
//
// @return 完整url
func ParseHttpRequest(req *http.Request) string {
	// TODO(chef): [refactor] scheme是否能从从req.URL.Scheme获取
	var scheme string
	if req.TLS == nil {
		scheme = "http"
	} else {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s%s", scheme, req.Host, req.RequestURI)
}

// ----- private -------------------------------------------------------------------------------------------------------

func parseUrlPath(stdUrl *url.URL) (ctx UrlPathContext, err error) {
	ctx.Path = stdUrl.Path

	index := strings.LastIndexByte(ctx.Path, '/')
	if index == -1 {
		ctx.PathWithoutLastItem = ""
		ctx.LastItemOfPath = ""
	} else if index == 0 {
		if ctx.Path == "/" {
			ctx.PathWithoutLastItem = ""
			ctx.LastItemOfPath = ""
		} else {
			ctx.PathWithoutLastItem = ""
			ctx.LastItemOfPath = ctx.Path[1:]
		}
	} else {
		ctx.PathWithoutLastItem = ctx.Path[1:index]
		ctx.LastItemOfPath = ctx.Path[index+1:]
	}

	ctx.RawQuery = stdUrl.RawQuery

	if ctx.RawQuery == "" {
		ctx.PathWithRawQuery = ctx.Path
	} else {
		ctx.PathWithRawQuery = fmt.Sprintf("%s?%s", ctx.Path, ctx.RawQuery)
	}

	return ctx, nil
}

func parseHttpUrl(rawUrl string, filetype string) (ctx UrlContext, err error) {
	ctx, err = ParseUrl(rawUrl, -1)
	if err != nil {
		return
	}
	if (ctx.Scheme != "http" && ctx.Scheme != "https") || ctx.Host == "" || ctx.Path == "" || !strings.HasSuffix(ctx.LastItemOfPath, filetype) {
		return ctx, fmt.Errorf("%w. url=%s", ErrInvalidUrl, rawUrl)
	}

	return
}
