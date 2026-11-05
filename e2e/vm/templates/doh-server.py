#!/usr/bin/env python3
"""Minimal DNS-over-HTTPS front end for the lab's dnsmasq upstream.

It exists so the suite can check whether a browser's own DoH resolver bypasses
the Sentinel's DNS filtering.
"""
import http.server
import socket
import ssl

UPSTREAM = ("192.168.150.30", 53)
LISTEN = ("192.168.150.30", 443)


class Handler(http.server.BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def do_POST(self):
        if self.path != "/dns-query":
            self.send_error(404)
            return
        query = self.rfile.read(int(self.headers.get("Content-Length", 0)))
        with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as upstream:
            upstream.settimeout(3)
            upstream.sendto(query, UPSTREAM)
            answer = upstream.recvfrom(65535)[0]
        self.send_response(200)
        self.send_header("Content-Type", "application/dns-message")
        self.send_header("Content-Length", str(len(answer)))
        self.end_headers()
        self.wfile.write(answer)


context = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
context.load_cert_chain("/etc/aegis-doh/doh.pem", "/etc/aegis-doh/doh.key")
server = http.server.ThreadingHTTPServer(LISTEN, Handler)
server.socket = context.wrap_socket(server.socket, server_side=True)
server.serve_forever()
