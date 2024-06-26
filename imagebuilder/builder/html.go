package builder

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/Cloud-Foundations/Dominator/imagebuilder/logarchiver"
	"github.com/Cloud-Foundations/Dominator/lib/filter"
	"github.com/Cloud-Foundations/Dominator/lib/format"
	"github.com/Cloud-Foundations/Dominator/lib/fsutil"
	"github.com/Cloud-Foundations/Dominator/lib/html"
	libjson "github.com/Cloud-Foundations/Dominator/lib/json"
	"github.com/Cloud-Foundations/Dominator/lib/stringutil"
)

const codeStyle = `background-color: #eee; border: 1px solid #999; display: block; float: left;`

func streamNameText(streamName string,
	autoRebuildStreams map[string]struct{}) string {
	if _, ok := autoRebuildStreams[streamName]; ok {
		return "<b>" + streamName + "</b>"
	} else {
		return streamName
	}
}

func writeFilter(writer io.Writer, prefix string, filt *filter.Filter) {
	if filt != nil && len(filt.FilterLines) > 0 {
		fmt.Fprintln(writer, prefix, "Filter lines:<br>")
		fmt.Fprintf(writer, "<pre style=\"%s\">\n", codeStyle)
		libjson.WriteWithIndent(writer, "    ", filt.FilterLines)
		fmt.Fprintln(writer, "</pre><p style=\"clear: both;\">")
	}
}

func (stream *bootstrapStream) WriteHtml(writer io.Writer) {
	fmt.Fprintf(writer, "Bootstrap command: <code>%s</code><br>\n",
		strings.Join(stream.BootstrapCommand, " "))
	writeFilter(writer, "", stream.Filter)
	packager := stream.builder.packagerTypes[stream.PackagerType]
	packager.WriteHtml(writer)
	writeFilter(writer, "Image ", stream.imageFilter)
	if stream.imageTriggers != nil {
		fmt.Fprintln(writer, "Image triggers:<br>")
		fmt.Fprintf(writer, "<pre style=\"%s\">\n", codeStyle)
		libjson.WriteWithIndent(writer, "    ", stream.imageTriggers.Triggers)
		fmt.Fprintln(writer, "</pre><p style=\"clear: both;\">")
	}
	if len(stream.imageTags) > 0 {
		fmt.Fprintln(writer, "Image tags:<br>")
		fmt.Fprintf(writer, "<pre style=\"%s\">\n", codeStyle)
		libjson.WriteWithIndent(writer, "    ", stream.imageTags)
		fmt.Fprintln(writer, "</pre><p style=\"clear: both;\">")
	}
}

func (b *Builder) getHtmlWriter(streamName string) html.HtmlWriter {
	if stream := b.getBootstrapStream(streamName); stream != nil {
		return stream
	}
	if stream := b.getNormalStream(streamName); stream != nil {
		return stream
	}
	// Ensure a nil interface is returned, not a stream with value == nil.
	return nil
}

func (b *Builder) showImageStream(writer io.Writer, streamName string) {
	stream := b.getHtmlWriter(streamName)
	if stream == nil {
		fmt.Fprintf(writer, "<b>Stream: %s does not exist!</b>\n", streamName)
		return
	}
	fmt.Fprintf(writer, "<h3>Information for stream: %s</h3>\n", streamName)
	stream.WriteHtml(writer)
}

func (b *Builder) showImageStreams(writer io.Writer) {
	streamNames := b.listAllStreamNames()
	sort.Strings(streamNames)
	fmt.Fprintln(writer, `<table border="1">`)
	tw, _ := html.NewTableWriter(writer, true,
		"Image Stream", "ManifestUrl", "ManifestDirectory")
	autoRebuildStreams := stringutil.ConvertListToMap(
		b.listStreamsToAutoRebuild(), false)
	for _, streamName := range streamNames {
		var manifestUrl, manifestDirectory string
		if imageStream := b.getNormalStream(streamName); imageStream != nil {
			manifestUrl = imageStream.ManifestUrl
			manifestDirectory = imageStream.ManifestDirectory
		}
		tw.WriteRow("", "",
			fmt.Sprintf("<a href=\"showImageStream?%s\">%s</a>",
				streamName, streamNameText(streamName, autoRebuildStreams)),
			manifestUrl, manifestDirectory)
	}
	tw.Close()
	fmt.Fprintln(writer, "<br>")
}

func (b *Builder) writeHtml(writer io.Writer) {
	fmt.Fprintf(writer,
		"Number of image streams: <a href=\"showImageStreams\">%d</a><br>\n",
		b.getNumStreams())
	fmt.Fprintln(writer,
		"Image stream <a href=\"showDirectedGraph\">relationships</a><br>")
	fmt.Fprintf(writer,
		"Image server: <a href=\"http://%s/\">%s</a><p>\n",
		b.imageServerAddress, b.imageServerAddress)
	currentBuildNames := make([]string, 0)
	currentBuildSlaves := make([]string, 0)
	currentBuildTimes := make([]time.Time, 0)
	goodBuilds := make(map[string]buildResultType)
	failedBuilds := make(map[string]buildResultType)
	var lastFailedBuild time.Time
	b.buildResultsLock.RLock()
	for name, info := range b.currentBuildInfos {
		currentBuildNames = append(currentBuildNames, name)
		currentBuildSlaves = append(currentBuildSlaves, info.slaveAddress)
		currentBuildTimes = append(currentBuildTimes, info.startedAt)
	}
	for name, result := range b.lastBuildResults {
		if result.error == nil {
			goodBuilds[name] = result
		} else {
			failedBuilds[name] = result
			if result.finishTime.After(lastFailedBuild) {
				lastFailedBuild = result.finishTime
			}
		}
	}
	b.buildResultsLock.RUnlock()
	autoRebuildStreams := stringutil.ConvertListToMap(
		b.listStreamsToAutoRebuild(), false)
	currentTime := time.Now()
	if len(currentBuildNames) > 0 {
		fmt.Fprintln(writer, "Current image builds:<br>")
		fmt.Fprintln(writer, `<table border="1">`)
		columnNames := []string{"Image Stream", "Build log", "Duration"}
		if b.slaveDriver != nil {
			columnNames = append(columnNames, "Slave")
		}
		tw, _ := html.NewTableWriter(writer, true, columnNames...)
		for index, streamName := range currentBuildNames {
			columns := []string{
				streamNameText(streamName, autoRebuildStreams),
				fmt.Sprintf("<a href=\"showCurrentBuildLog?%s#bottom\">log</a>",
					streamName),
				format.Duration(time.Since(currentBuildTimes[index])),
			}
			if b.slaveDriver != nil {
				var slaveColumn string
				if address := currentBuildSlaves[index]; address != "" {
					host, _, err := net.SplitHostPort(address)
					if err != nil {
						host = address
					}
					slaveColumn = fmt.Sprintf("<a href=\"http://%s/\">%s</a>",
						address, host)
				}
				columns = append(columns, slaveColumn)
			}
			tw.WriteRow("", "", columns...)
		}
		tw.Close()
		fmt.Fprintln(writer, "<br>")
	}
	if len(failedBuilds) > 0 {
		streamNames := make([]string, 0, len(failedBuilds))
		for streamName := range failedBuilds {
			streamNames = append(streamNames, streamName)
		}
		sort.Strings(streamNames)
		fmt.Fprintf(writer, "Failed image builds: (last at: %s, %s ago)<br>\n",
			lastFailedBuild.Format(format.TimeFormatSeconds),
			format.Duration(time.Since(lastFailedBuild)))
		fmt.Fprintln(writer, `<table border="1">`)
		tw, _ := html.NewTableWriter(writer, true,
			"Image Stream", "Error", "Build log", "Duration", "Last attempt")
		for _, streamName := range streamNames {
			result := failedBuilds[streamName]
			tw.WriteRow("", "",
				streamNameText(streamName, autoRebuildStreams),
				result.error.Error(),
				fmt.Sprintf("<a href=\"showLastBuildLog?%s\">log</a>",
					streamName),
				format.Duration(result.finishTime.Sub(result.startTime)),
				fmt.Sprintf("%s ago",
					format.Duration(currentTime.Sub(result.finishTime))),
			)
		}
		tw.Close()
		fmt.Fprintln(writer, "<br>")
	}
	if len(goodBuilds) > 0 {
		streamNames := make([]string, 0, len(goodBuilds))
		for streamName := range goodBuilds {
			streamNames = append(streamNames, streamName)
		}
		sort.Strings(streamNames)
		fmt.Fprintln(writer, "Successful image builds:<br>")
		fmt.Fprintln(writer, `<table border="1">`)
		tw, _ := html.NewTableWriter(writer, true, "Image Stream", "Name",
			"Build log", "Duration", "Age")
		for _, streamName := range streamNames {
			result := goodBuilds[streamName]
			tw.WriteRow("", "",
				streamNameText(streamName, autoRebuildStreams),
				fmt.Sprintf("<a href=\"http://%s/showImage?%s\">%s</a>",
					b.linksImageServerAddress, result.imageName,
					result.imageName),
				fmt.Sprintf("<a href=\"showLastBuildLog?%s\">log</a>",
					streamName),
				format.Duration(result.finishTime.Sub(result.startTime)),
				fmt.Sprintf("%s ago",
					format.Duration(currentTime.Sub(result.finishTime))),
			)
		}
		tw.Close()
		fmt.Fprintln(writer, "<br>")
	}
	if _, ok := b.buildLogArchiver.(logarchiver.BuildLogReporter); ok {
		fmt.Fprintln(writer,
			"Build log <a href=\"showBuildLogArchive\">archive</a><br>")
	}
}

func (stream *imageStreamType) WriteHtml(writer io.Writer) {
	if len(stream.BuilderGroups) > 0 {
		fmt.Fprintf(writer, "BuilderGroups: %s<br>\n",
			strings.Join(stream.BuilderGroups, ", "))
	}
	if len(stream.BuilderUsers) > 0 {
		fmt.Fprintf(writer, "BuilderUsers: %s<br>\n",
			strings.Join(stream.BuilderUsers, ", "))
	}
	manifestLocation := stream.getManifestLocation(nil, nil)
	fmt.Fprintf(writer, "Manifest URL: <code>%s</code><br>\n",
		stream.ManifestUrl)
	if manifestLocation.url != stream.ManifestUrl {
		fmt.Fprintf(writer, "Manifest URL (expanded): <code>%s</code><br>\n",
			manifestLocation.url)
	}
	fmt.Fprintf(writer, "Manifest Directory: <code>%s</code><br>\n",
		stream.ManifestDirectory)
	if manifestLocation.directory != stream.ManifestDirectory {
		fmt.Fprintf(writer,
			"Manifest Directory (expanded): <code>%s</code><br>\n",
			manifestLocation.directory)
	}
	buildLog := new(bytes.Buffer)
	manifestDirectory, sourceImageName, gitInfo, manifestBytes, _, err :=
		stream.getSourceImage(stream.builder, buildLog)
	if err != nil {
		fmt.Fprintf(writer, "<b>%s</b><br>\n", err)
		return
	}
	defer os.RemoveAll(manifestDirectory)
	if gitInfo != nil {
		fmt.Fprintf(writer,
			"Latest commit on branch: <code>%s</code>: <code>%s</code>s<br>\n",
			gitInfo.branch, gitInfo.commitId)
	}
	if stream.builder.getHtmlWriter(sourceImageName) == nil {
		fmt.Fprintf(writer, "SourceImage: <code>%s</code><br>\n",
			sourceImageName)
	} else {
		fmt.Fprintf(writer,
			"SourceImage: <a href=\"showImageStream?%s\"><code>%s</code></a><br>\n",
			sourceImageName, sourceImageName)
	}
	if len(stream.Variables) > 0 {
		fmt.Fprintln(writer, "Stream variables:<br>")
		fmt.Fprintf(writer, "<pre style=\"%s\">\n", codeStyle)
		libjson.WriteWithIndent(writer, "    ", stream.Variables)
		fmt.Fprintln(writer, "</pre><p style=\"clear: both;\">")
	}
	fmt.Fprintln(writer, "Contents of <code>manifest</code> file:<br>")
	fmt.Fprintf(writer, "<pre style=\"%s\">\n", codeStyle)
	writer.Write(manifestBytes)
	fmt.Fprintln(writer, "</pre><p style=\"clear: both;\">")
	packagesFile, err := os.Open(path.Join(manifestDirectory, "package-list"))
	if err == nil {
		defer packagesFile.Close()
		fmt.Fprintln(writer, "Contents of <code>package-list</code> file:<br>")
		fmt.Fprintf(writer, "<pre style=\"%s\">\n", codeStyle)
		io.Copy(writer, packagesFile)
		fmt.Fprintln(writer, "</pre><p style=\"clear: both;\">")
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(writer, "<b>%s</b><br>\n", err)
		return
	}
	tagsFile, err := os.Open(path.Join(manifestDirectory, "tags.json"))
	if err == nil {
		defer tagsFile.Close()
		fmt.Fprintln(writer, "Contents of <code>tags.json</code> file:<br>")
		fmt.Fprintf(writer, "<pre style=\"%s\">\n", codeStyle)
		io.Copy(writer, tagsFile)
		fmt.Fprintln(writer, "</pre><p style=\"clear: both;\">")
	} else if !os.IsNotExist(err) {
		fmt.Fprintf(writer, "<b>%s</b><br>\n", err)
		return
	}
	if size, err := fsutil.GetTreeSize(manifestDirectory); err != nil {
		fmt.Fprintf(writer, "<b>%s</b><br>\n", err)
		return
	} else {
		fmt.Fprintf(writer, "Manifest tree size: %s<br>\n",
			format.FormatBytes(size))
	}
	fmt.Fprintln(writer, "<hr style=\"height:2px\"><font color=\"#bbb\">")
	fmt.Fprintln(writer, "<b>Logging output:</b>")
	fmt.Fprintln(writer, "<pre>")
	io.Copy(writer, buildLog)
	fmt.Fprintln(writer, "</pre>")
	fmt.Fprintln(writer, "</font>")
}

func (packager *packagerType) WriteHtml(writer io.Writer) {
	fmt.Fprintf(writer, "Clean command: <code>%s</code><br>\n",
		strings.Join(packager.CleanCommand, " "))
	fmt.Fprintf(writer, "Install command: <code>%s</code><br>\n",
		strings.Join(packager.InstallCommand, " "))
	fmt.Fprintf(writer, "List command: <code>%s</code><br>\n",
		strings.Join(packager.ListCommand.ArgList, " "))
	if packager.ListCommand.SizeMultiplier > 1 {
		fmt.Fprintf(writer, "List command size multiplier: %d<br>\n",
			packager.ListCommand.SizeMultiplier)
	}
	fmt.Fprintf(writer, "Remove command: <code>%s</code><br>\n",
		strings.Join(packager.RemoveCommand, " "))
	fmt.Fprintf(writer, "Update command: <code>%s</code><br>\n",
		strings.Join(packager.UpdateCommand, " "))
	fmt.Fprintf(writer, "Upgrade command: <code>%s</code><br>\n",
		strings.Join(packager.UpgradeCommand, " "))
	if len(packager.Verbatim) > 0 {
		fmt.Fprintln(writer, "Verbatim lines:<br>")
		fmt.Fprintf(writer, "<pre style=\"%s\">\n", codeStyle)
		libjson.WriteWithIndent(writer, "    ", packager.Verbatim)
		fmt.Fprintln(writer, "</pre><p style=\"clear: both;\">")
	}
	fmt.Fprintln(writer, "Package installer script:<br>")
	fmt.Fprintf(writer, "<pre style=\"%s\">\n", codeStyle)
	packager.writePackageInstallerContents(writer)
	fmt.Fprintln(writer, "</pre><p style=\"clear: both;\">")
}
