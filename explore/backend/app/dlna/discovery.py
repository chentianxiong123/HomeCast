import asyncio
import select
import socket
import struct
import time
import threading
import xml.etree.ElementTree as ET
from concurrent.futures import ThreadPoolExecutor
from dataclasses import dataclass
from typing import Callable
from urllib.request import urlopen, Request
from urllib.error import URLError
from loguru import logger


_executor = ThreadPoolExecutor(max_workers=2)


@dataclass
class DLNADevice:
    name: str
    udn: str
    location: str
    ip: str
    port: int
    device_type: str = ""
    services: list = None

    def __post_init__(self):
        if self.services is None:
            self.services = []


class DLNADiscovery:
    SSDP_ADDR = "239.255.255.250"
    SSDP_PORT = 1900
    SSDP_MX = 3

    SSDP_SEARCH_MSG = (
        "M-SEARCH * HTTP/1.1\r\n"
        "HOST: 239.255.255.250:1900\r\n"
        'MAN: "ssdp:discover"\r\n'
        "MX: {mx}\r\n"
        "ST: {st}\r\n"
        "\r\n"
    )

    def __init__(self):
        self.devices: dict[str, DLNADevice] = {}
        self._callback: Callable | None = None

    def set_callback(self, callback: Callable):
        self._callback = callback

    async def search(self, timeout: float = 5.0, target_ip: str = None) -> list[DLNADevice]:
        logger.info(f"Searching DLNA devices (timeout={timeout}s, target_ip={target_ip})")
        self.devices.clear()

        def _get_local_ip() -> str:
            s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
            try:
                s.connect(("8.8.8.8", 80))
                ip = s.getsockname()[0]
            except Exception:
                ip = "0.0.0.0"
            finally:
                s.close()
            return ip

        def _search_sync():
            local_ip = _get_local_ip()
            logger.info(f"Local IP: {local_ip}")

            # 方案1: 多播+接收
            sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM, socket.IPPROTO_UDP)
            sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
            sock.settimeout(0.5)

            try:
                sock.bind((local_ip, 1900))
                mreq = struct.pack('4s4s', socket.inet_aton(self.SSDP_ADDR), socket.inet_aton(local_ip))
                sock.setsockopt(socket.IPPROTO_IP, socket.IP_ADD_MEMBERSHIP, mreq)
                logger.info("Joined multicast group")
            except Exception as e:
                logger.warning(f"Failed to bind/join multicast: {e}")

            search_types = [
                "urn:schemas-upnp-org:device:MediaRenderer:1",
                "urn:schemas-upnp-org:service:AVTransport:1",
                "upnp:rootdevice",
            ]

            # 发送多播搜索
            for st in search_types:
                msg = self.SSDP_SEARCH_MSG.format(mx=self.SSDP_MX, st=st).encode()
                try:
                    sock.sendto(msg, (self.SSDP_ADDR, self.SSDP_PORT))
                except Exception as e:
                    logger.warning(f"Multicast send failed: {e}")

            # 接收响应 - 先快速收集基本信息，暂不请求XML
            start_time = time.time()
            found = 0
            pending_devices = []  # 存储需要获取friendlyName的设备

            # 方案2: 如果指定了target_ip，发送unicast
            if target_ip:
                logger.info(f"Sending unicast to {target_ip}:1900")
                unicast_sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM, socket.IPPROTO_UDP)
                unicast_sock.settimeout(0.5)
                for st in search_types:
                    msg = self.SSDP_SEARCH_MSG.format(mx=1, st=st).encode()
                    try:
                        unicast_sock.sendto(msg, (target_ip, 1900))
                        logger.info(f"Unicast sent to {target_ip}")
                    except Exception as e:
                        logger.warning(f"Unicast send failed: {e}")
                # 不要立即关闭，等待响应
                try:
                    unicast_sock.setblocking(False)
                    import select
                    start = time.time()
                    unicast_found = 0
                    while time.time() - start < 3:
                        read, _, _ = select.select([unicast_sock], [], [], 0.5)
                        if read:
                            data, addr = unicast_sock.recvfrom(2048)
                            if addr[0] != local_ip:
                                unicast_found += 1
                                logger.info(f"Unicast received from {addr}: {len(data)} bytes")
                                device_info = self._parse_device_info(data, addr)
                                if device_info:
                                    pending_devices.append(device_info)
                        time.sleep(0.1)
                    if unicast_found > 0:
                        logger.info(f"Unicast found {unicast_found} devices")
                except Exception as e:
                    logger.debug(f"Unicast receive: {e}")
                finally:
                    unicast_sock.close()
            while time.time() - start_time < timeout:
                try:
                    data, addr = sock.recvfrom(2048)
                    # 忽略我们自己的echo
                    if addr[0] == local_ip and b"M-SEARCH" in data:
                        continue
                    found += 1
                    logger.info(f"Received from {addr}: {len(data)} bytes")
                    device_info = self._parse_device_info(data, addr)
                    if device_info:
                        pending_devices.append(device_info)
                except socket.timeout:
                    continue
                except Exception as e:
                    logger.warning(f"SSDP receive error: {e}")

            sock.close()
            logger.info(f"SSDP search complete, received {found} responses, {len(pending_devices)} unique devices")

            # 并行获取所有设备的friendlyName
            if pending_devices:
                self._fetch_all_friendly_names(pending_devices)

        loop = asyncio.get_event_loop()
        await loop.run_in_executor(_executor, _search_sync)
        logger.info(f"Found {len(self.devices)} DLNA devices")
        return list(self.devices.values())

    def _fetch_friendly_name(self, location: str) -> str | None:
        """Fetch device description XML and extract friendlyName"""
        try:
            req = Request(location, headers={"User-Agent": "PlayOn DLNA Controller"})
            with urlopen(req, timeout=2) as response:
                xml_data = response.read()
                root = ET.fromstring(xml_data)
                ns = {"upnp": "urn:schemas-upnp-org:device-1-0"}
                friendly_name = root.find(".//upnp:friendlyName", ns)
                if friendly_name is not None and friendly_name.text:
                    return friendly_name.text
                friendly_name = root.find(".//friendlyName")
                if friendly_name is not None and friendly_name.text:
                    return friendly_name.text
        except Exception as e:
            logger.debug(f"Failed to get friendlyName from {location}: {e}")
        return None

    def _fetch_all_friendly_names(self, pending_devices: list):
        """并行获取所有设备的friendlyName"""
        import concurrent.futures
        with concurrent.futures.ThreadPoolExecutor(max_workers=5) as executor:
            future_to_device = {
                executor.submit(self._fetch_friendly_name, dev["location"]): dev
                for dev in pending_devices
            }
            for future in concurrent.futures.as_completed(future_to_device):
                dev = future_to_device[future]
                try:
                    friendly_name = future.result()
                    if not friendly_name:
                        friendly_name = dev.get("server_name", "Unknown")
                    device = DLNADevice(
                        name=friendly_name,
                        udn=dev["udn"],
                        location=dev["location"],
                        ip=dev["ip"],
                        port=dev["port"],
                        device_type=dev["device_type"],
                    )
                    self.devices[dev["udn"]] = device
                    logger.info(f"Found DLNA device: {device.name} at {device.ip}:{device.port}")
                    if self._callback:
                        self._callback(device)
                except Exception as e:
                    logger.warning(f"Failed to process device {dev.get('udn')}: {e}")

    def _parse_device_info(self, data: bytes, addr: tuple) -> dict | None:
        """解析设备基本信息，返回临时dict"""
        try:
            text = data.decode("utf-8", errors="ignore")
            headers = self._parse_headers(text)
            location = headers.get("LOCATION", "")
            st = headers.get("ST", "")
            usn = headers.get("USN", "")

            if not location or not usn:
                return None

            udn = usn.split("::")[0]
            if udn in self.devices:
                return None

            ip, port = self._parse_location(location)
            server_name = headers.get("SERVER", "Unknown").split("/")[0]

            return {
                "udn": udn,
                "location": location,
                "ip": ip,
                "port": port,
                "device_type": st,
                "server_name": server_name,
            }
        except Exception as e:
            logger.debug(f"Parse device info failed: {e}")
            return None

    def _handle_response_data(self, data: bytes, addr: tuple):
        try:
            text = data.decode("utf-8", errors="ignore")
            headers = self._parse_headers(text)

            location = headers.get("LOCATION", "")
            st = headers.get("ST", "")
            usn = headers.get("USN", "")

            if not location or not usn:
                return

            udn = usn.split("::")[0]
            if udn in self.devices:
                return

            ip, port = self._parse_location(location)

            # Try to get friendlyName from device description XML
            friendly_name = self._fetch_friendly_name(location)
            if not friendly_name:
                # Fallback to SERVER header
                friendly_name = headers.get("SERVER", "Unknown").split("/")[0]

            device = DLNADevice(
                name=friendly_name,
                udn=udn,
                location=location,
                ip=ip,
                port=port,
                device_type=st,
            )

            self.devices[udn] = device
            logger.info(f"Found DLNA device: {device.name} at {ip}:{port}")

            if self._callback:
                self._callback(device)

        except Exception as e:
            logger.warning(f"Parse SSDP response failed: {e}")

    def _parse_headers(self, text: str) -> dict:
        headers = {}
        lines = text.strip().split("\r\n")
        for line in lines[1:]:
            if ":" in line:
                key, value = line.split(":", 1)
                headers[key.upper().strip()] = value.strip()
        return headers

    def _parse_location(self, location: str) -> tuple[str, int]:
        try:
            if location.startswith("http://"):
                location = location[7:]
            parts = location.split("/", 1)[0].split(":")
            ip = parts[0]
            port = int(parts[1]) if len(parts) > 1 else 80
            return ip, port
        except Exception:
            return "", 80
