# How velo measures

This page describes each value that velo shows, how velo measures it, and the limits of the value.

## Server selection

1. velo requests the 10 nearest servers from `https://www.speedtest.net/api/js/servers?engine=js&https_functional=true&limit=10`.
2. velo sorts the servers by distance.
3. velo sends all test requests to the `host` of each server. The `url` from the list gives only the scheme and the path. Most `url` hosts answer with a `307` redirect to the `host`. Go does not follow a redirect for an upload body.
4. velo opens one connection to each server and sends 3 requests for `latency.txt`. The first request opens the connection. velo keeps the lower time of the other 2 requests.
5. velo selects the server with the lowest time. When no server answers, velo selects the nearest server.

At the same time, velo requests `https://www.speedtest.net/speedtest-config.php` for your IP address and your ISP.

To use one server, give its ID with `--server`. The server list in velo shows the IDs.

## Latency phase

velo sends 24 HTTP `GET` requests for `<server>/speedtest/latency.txt`. All requests use one keep-alive connection, which velo opens before the first probe. velo starts a probe every 100 ms. When a probe takes longer than 100 ms, the next probe starts when that probe ends.

| Value | Calculation |
| --- | --- |
| Ping | The mean of the probe times. |
| Jitter | The mean of the absolute differences between 2 probes in sequence. |

A probe that takes longer than 2 seconds counts as lost.

## Download phase

velo opens 16 parallel HTTP streams for 8 seconds. Each stream requests `<server>/speedtest/random4000x4000.jpg` again and again, and reads the body. velo counts the bytes that it reads.

## Upload phase

velo opens 16 parallel HTTP streams for 7 seconds. Each stream sends `POST` requests of 8 MiB to `<server>/speedtest/upload.php`.

On Linux, velo counts the bytes that the server acknowledged. velo reads `tcpi_bytes_acked` from `TCP_INFO` on each connection. A count of written bytes also includes data that only waits in the socket buffers. That count shows a false peak when the streams open.

On other systems, velo counts the bytes that it writes.

A server that rejects an upload request still acknowledges the start of the body. When no request gets `200 OK` and 16 or more requests get another status, the upload phase fails with that status. Example: `upload failed · 307 Temporary Redirect`.

## Throughput samples

velo takes a sample every 50 ms. Each sample is the throughput over the last 250 ms. The download phase has 160 samples. The upload phase has 140 samples.

| Value | Calculation |
| --- | --- |
| Result | The mean of the samples after the first 30 % of the phase. This removes the TCP ramp. |
| Peak | The highest sample. |
| Live readout | A moving average of the samples. Each new sample has a weight of 0.3. |
| Data used | All bytes of both phases. |

## Loaded latency and bufferbloat

During download and upload, velo sends a latency probe every 250 ms on the same connection as the latency phase. These probes wait in the same queues as the test traffic. A probe that takes longer than 3 seconds counts as lost.

| Value | Calculation |
| --- | --- |
| Loaded ▼ | The mean probe time during download. |
| Loaded ▲ | The mean probe time during upload. |
| Bufferbloat | The higher loaded value minus ping. |
| Grade | The grade of the bufferbloat. See the table below. |

| Bufferbloat | Grade | Meaning |
| --- | --- | --- |
| less than 5 ms | A+ | No queueing under load |
| less than 30 ms | A | Minimal queueing |
| less than 60 ms | B | Noticeable on calls |
| less than 200 ms | C | Lag under load |
| 200 ms or more | D | Severe bufferbloat |

## Loss

LOSS is the share of all latency probes that failed or took longer than the timeout. velo uses HTTP over TCP. TCP sends lost packets again, so packet loss shows as more latency and not as lost probes. LOSS is therefore not the packet loss of the link.

## Gauge scale

The gauge uses the scale 0, 5, 10, 25, 50, 100, 250, 500, and 1G Mbps. Each segment between 2 labels has the same length. When a sample is higher than 1 Gbps, velo changes to a scale that ends at 2.5G. When a sample is higher than 2.5 Gbps, velo changes to a scale that ends at 10G.

## Requests

velo sends the header `User-Agent: velo/<version>` with each request. velo uses the proxy from `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY`.
