package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jhump/protoreflect/desc"
	"github.com/jhump/protoreflect/desc/protoparse"
	"github.com/jhump/protoreflect/dynamic"
)

func main() {
	baseURL := flag.String("url", "http://127.0.0.1:8080", "Base URL with optional path prefix")
	method := flag.String("method", "", "Service/Method to call (e.g., FakeService/Handle)")
	protoFile := flag.String("proto", "", "Path to .proto file (enables JSON input)")
	jsonData := flag.String("json", "", "JSON request data (requires -proto)")
	stream := flag.Bool("stream", false, "Enable server streaming mode")
	timeout := flag.Duration("timeout", 30*time.Second, "Request timeout")
	flag.Parse()

	if *method == "" {
		printUsage()
		os.Exit(1)
	}

	// Parse method into service and method name
	parts := strings.Split(*method, "/")
	if len(parts) != 2 {
		fmt.Fprintf(os.Stderr, "Error: method must be in format Service/Method\n")
		os.Exit(1)
	}
	serviceName, methodName := parts[0], parts[1]

	// Build request payload
	var payload []byte
	var responseDesc *desc.MessageDescriptor

	if *protoFile != "" {
		// Load proto and encode JSON to protobuf
		var err error
		payload, responseDesc, err = encodeFromProto(*protoFile, serviceName, methodName, *jsonData)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error encoding request: %v\n", err)
			os.Exit(1)
		}
	}

	// Build URL
	url := strings.TrimSuffix(*baseURL, "/") + "/" + strings.TrimPrefix(*method, "/")

	// Create gRPC-Web request
	body := makeGrpcWebFrame(payload)

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating request: %v\n", err)
		os.Exit(1)
	}

	req.Header.Set("Content-Type", "application/grpc-web+proto")
	req.Header.Set("X-Grpc-Web", "1")
	req.Header.Set("Accept", "application/grpc-web+proto")

	// Send request
	client := &http.Client{Timeout: *timeout}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error sending request: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	// Print request info
	fmt.Printf("URL: %s\n", url)
	fmt.Printf("HTTP Status: %d\n", resp.StatusCode)

	if grpcStatus := resp.Header.Get("Grpc-Status"); grpcStatus != "" {
		fmt.Printf("grpc-status: %s\n", grpcStatus)
	}
	if grpcMessage := resp.Header.Get("Grpc-Message"); grpcMessage != "" {
		fmt.Printf("grpc-message: %s\n", grpcMessage)
	}

	fmt.Println("\n--- Response ---")

	if *stream {
		// Streaming mode - read frames as they arrive
		readStreamingResponse(resp.Body, responseDesc)
	} else {
		// Unary mode - read all at once
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading response: %v\n", err)
			os.Exit(1)
		}
		parseGrpcWebResponse(respBody, responseDesc)
	}
}

func printUsage() {
	fmt.Fprintln(os.Stderr, "grpcwebcli - gRPC-Web client with path prefix support")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Usage:")
	fmt.Fprintln(os.Stderr, "  grpcwebcli -url <url> -method <Service/Method> [options]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Options:")
	flag.PrintDefaults()
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Examples:")
	fmt.Fprintln(os.Stderr, "  # Simple call (empty request)")
	fmt.Fprintln(os.Stderr, "  grpcwebcli -url http://127.0.0.1:8080/api -method FakeService/Handle")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  # With proto file and JSON data")
	fmt.Fprintln(os.Stderr, "  grpcwebcli -url http://127.0.0.1:8080/api -method FakeService/Handle \\")
	fmt.Fprintln(os.Stderr, "    -proto api.proto -json '{\"data\": \"dGVzdA==\"}'")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "  # Server streaming")
	fmt.Fprintln(os.Stderr, "  grpcwebcli -url http://127.0.0.1:8080/api -method grpc.health.v1.Health/Watch \\")
	fmt.Fprintln(os.Stderr, "    -proto health.proto -stream")
}

func encodeFromProto(protoPath, serviceName, methodName, jsonData string) ([]byte, *desc.MessageDescriptor, error) {
	// Parse proto file
	parser := protoparse.Parser{
		ImportPaths: []string{filepath.Dir(protoPath), "."},
	}

	fds, err := parser.ParseFiles(filepath.Base(protoPath))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse proto: %w", err)
	}

	if len(fds) == 0 {
		return nil, nil, fmt.Errorf("no file descriptors parsed")
	}

	fd := fds[0]

	// Find service and method
	var methodDesc *desc.MethodDescriptor
	for _, svc := range fd.GetServices() {
		// Match service name (with or without package)
		svcFullName := svc.GetFullyQualifiedName()
		svcSimpleName := svc.GetName()

		if svcFullName == serviceName || svcSimpleName == serviceName {
			methodDesc = svc.FindMethodByName(methodName)
			if methodDesc != nil {
				break
			}
		}
	}

	if methodDesc == nil {
		return nil, nil, fmt.Errorf("method %s/%s not found in proto", serviceName, methodName)
	}

	// Get input message descriptor
	inputDesc := methodDesc.GetInputType()
	outputDesc := methodDesc.GetOutputType()

	// Create dynamic message
	msg := dynamic.NewMessage(inputDesc)

	// Parse JSON into message if provided
	if jsonData != "" {
		if err := msg.UnmarshalJSON([]byte(jsonData)); err != nil {
			return nil, nil, fmt.Errorf("failed to parse JSON: %w", err)
		}
	}

	// Marshal to protobuf
	payload, err := msg.Marshal()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal protobuf: %w", err)
	}

	return payload, outputDesc, nil
}

func makeGrpcWebFrame(payload []byte) []byte {
	frame := make([]byte, 5+len(payload))
	frame[0] = 0 // not compressed
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(payload)))
	copy(frame[5:], payload)
	return frame
}

func readStreamingResponse(r io.Reader, responseDesc *desc.MessageDescriptor) {
	buf := make([]byte, 5)
	frameNum := 0

	for {
		// Read frame header
		_, err := io.ReadFull(r, buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Printf("(stream ended: %v)\n", err)
			break
		}

		flags := buf[0]
		length := binary.BigEndian.Uint32(buf[1:5])

		// Read frame body
		frameData := make([]byte, length)
		_, err = io.ReadFull(r, frameData)
		if err != nil {
			fmt.Printf("(incomplete frame: %v)\n", err)
			break
		}

		printFrameContent(frameNum, flags, frameData, responseDesc)
		frameNum++

		// Check for trailers (end of stream)
		if flags&0x80 != 0 {
			break
		}
	}
}

func parseGrpcWebResponse(data []byte, responseDesc *desc.MessageDescriptor) {
	offset := 0
	frameNum := 0

	for offset < len(data) {
		if offset+5 > len(data) {
			fmt.Printf("(incomplete frame header at offset %d)\n", offset)
			break
		}

		flags := data[offset]
		length := binary.BigEndian.Uint32(data[offset+1 : offset+5])
		offset += 5

		if offset+int(length) > len(data) {
			fmt.Printf("(incomplete frame body, expected %d bytes)\n", length)
			remaining := data[offset:]
			printFrameContent(frameNum, flags, remaining, responseDesc)
			break
		}

		frameData := data[offset : offset+int(length)]
		offset += int(length)

		printFrameContent(frameNum, flags, frameData, responseDesc)
		frameNum++
	}
}

func printFrameContent(frameNum int, flags byte, data []byte, responseDesc *desc.MessageDescriptor) {
	frameType := "DATA"
	if flags&0x80 != 0 {
		frameType = "TRAILERS"
	}

	fmt.Printf("[Frame %d] %s (len=%d)\n", frameNum, frameType, len(data))

	if flags&0x80 != 0 {
		// Trailers - print as text
		fmt.Printf("%s\n", string(data))
		return
	}

	if len(data) == 0 {
		return
	}

	// Try to decode with proto descriptor if available
	if responseDesc != nil {
		msg := dynamic.NewMessage(responseDesc)
		if err := msg.Unmarshal(data); err == nil {
			jsonBytes, err := msg.MarshalJSON()
			if err == nil {
				fmt.Println(string(jsonBytes))
				return
			}
		}
	}

	// Fallback: try to extract readable strings from protobuf
	extracted := extractStrings(data)
	if extracted != "" {
		fmt.Println(extracted)
	} else {
		fmt.Printf("(binary, %d bytes): %x\n", len(data), data)
	}
}

func extractStrings(data []byte) string {
	var result strings.Builder
	i := 0

	for i < len(data) {
		if i >= len(data) {
			break
		}

		tag := data[i]
		wireType := tag & 0x07
		i++

		if wireType == 2 {
			length, bytesRead := readVarint(data[i:])
			i += bytesRead

			if i+int(length) <= len(data) {
				content := data[i : i+int(length)]
				i += int(length)

				if isPrintable(content) {
					if result.Len() > 0 {
						result.WriteString("\n")
					}
					result.Write(content)
				}
			} else {
				break
			}
		} else if wireType == 0 {
			_, bytesRead := readVarint(data[i:])
			i += bytesRead
		} else {
			break
		}
	}

	return result.String()
}

func readVarint(data []byte) (uint64, int) {
	var result uint64
	var shift uint
	for i, b := range data {
		result |= uint64(b&0x7F) << shift
		if b&0x80 == 0 {
			return result, i + 1
		}
		shift += 7
		if i >= 9 {
			break
		}
	}
	return result, len(data)
}

func isPrintable(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	printableCount := 0
	for _, b := range data {
		if (b >= 32 && b < 127) || b == '\n' || b == '\r' || b == '\t' {
			printableCount++
		}
	}
	return float64(printableCount)/float64(len(data)) > 0.8
}
