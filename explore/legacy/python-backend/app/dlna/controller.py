import xml.etree.ElementTree as ET
from typing import Any
from urllib.parse import urljoin
from loguru import logger
import requests
from app.dlna.discovery import DLNADevice


def build_didl_lite(uri: str, title: str = "Video", content_type: str = "video/mp4") -> str:
    upnp_class = "object.item.videoItem"
    if content_type.startswith("audio"):
        upnp_class = "object.item.audioItem"

    mime = content_type or "video/mp4"

    protocol_info = (
        f"http-get:*:{mime}:"
        "DLNA.ORG_PN=;DLNA.ORG_OP=01;DLNA.ORG_CI=0;DLNA.ORG_FLAGS=01700000000000000000000000000000"
    )

    escaped_title = (
        title.replace("&", "&amp;")
        .replace("<", "&lt;")
        .replace(">", "&gt;")
        .replace('"', "&quot;")
    )
    escaped_uri = uri.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;")

    return (
        '<DIDL-Lite xmlns:dc="http://purl.org/dc/elements/1.1/" '
        'xmlns:dlna="urn:schemas-dlna-org:metadata-1-0/" '
        'xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" '
        'xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/">'
        '<item id="0" parentID="-1" restricted="1">'
        f'<upnp:class>{upnp_class}</upnp:class>'
        f'<dc:title>{escaped_title}</dc:title>'
        f'<res protocolInfo="{protocol_info}">{escaped_uri}</res>'
        '</item></DIDL-Lite>'
    )


class DLNAController:
    def __init__(self, device: DLNADevice):
        self.device = device
        self.av_transport_url: str = ""
        self.rendering_control_url: str = ""

    def init(self):
        logger.info(f"Initializing controller for device: {self.device.name} at {self.device.location}")
        self._parse_device_description()
        logger.info(f"AVTransport URL: {self.av_transport_url}, RenderingControl URL: {self.rendering_control_url}")

    def _parse_device_description(self):
        try:
            resp = requests.get(self.device.location, timeout=10, headers={"User-Agent": "PlayOn DLNA Controller"})
            resp.raise_for_status()
            root = ET.fromstring(resp.text)

            for service in root.iter("{urn:schemas-upnp-org:device-1-0}service"):
                service_type = service.find("{urn:schemas-upnp-org:device-1-0}serviceType")
                control_url = service.find("{urn:schemas-upnp-org:device-1-0}controlURL")

                if service_type is None or control_url is None:
                    continue

                st = service_type.text or ""
                url = control_url.text or ""

                base_url = f"http://{self.device.ip}:{self.device.port}"
                full_url = urljoin(base_url, url)

                if "AVTransport" in st:
                    self.av_transport_url = full_url
                    logger.debug(f"AVTransport URL: {full_url}")
                elif "RenderingControl" in st:
                    self.rendering_control_url = full_url
                    logger.debug(f"RenderingControl URL: {full_url}")

        except Exception as e:
            logger.error(f"Parse device description failed: {e}")

    def _build_soap_body(self, service: str, action: str, params: dict) -> str:
        ns = {
            "AVTransport": "urn:schemas-upnp-org:service:AVTransport:1",
            "RenderingControl": "urn:schemas-upnp-org:service:RenderingControl:1",
        }
        service_ns = ns.get(service, f"urn:schemas-upnp-org:service:{service}:1")

        param_xml = "\n".join(
            f"<{k}>{self._escape_xml(str(v))}</{k}>" for k, v in params.items()
        )

        return f"""<?xml version="1.0" encoding="utf-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <s:Body>
    <u:{action} xmlns:u="{service_ns}">
      {param_xml}
    </u:{action}>
  </s:Body>
</s:Envelope>"""

    def _escape_xml(self, text: str) -> str:
        return (
            text.replace("&", "&amp;")
            .replace("<", "&lt;")
            .replace(">", "&gt;")
            .replace('"', "&quot;")
            .replace("'", "&apos;")
        )

    def _parse_soap_response(self, xml_text: str, tag_name: str) -> str | None:
        try:
            root = ET.fromstring(xml_text)
            for elem in root.iter():
                local = elem.tag.split("}")[-1] if "}" in elem.tag else elem.tag
                if local == tag_name:
                    return elem.text
        except ET.ParseError:
            pass
        return None

    def _send_soap(self, url: str, service: str, action: str, body: str) -> dict | None:
        try:
            headers = {
                "Content-Type": "text/xml; charset=utf-8",
                "SOAPACTION": f'"urn:schemas-upnp-org:service:{service}:1#{action}"',
                "User-Agent": "PlayOn DLNA Controller",
            }
            resp = requests.post(url, data=body.encode(), headers=headers, timeout=30)
            resp.raise_for_status()
            return {"status": "ok", "response": resp.text}
        except requests.HTTPError as e:
            logger.error(f"SOAP HTTP error {e.response.status_code} for {action}: {e.response.text[:200]}")
            return None
        except Exception as e:
            logger.error(f"SOAP request failed for {action}: {e}")
            return None

    def set_av_transport_uri(self, uri: str, title: str = "Video", content_type: str = "video/mp4") -> bool:
        if not self.av_transport_url:
            logger.error("AVTransport URL not available")
            return False

        metadata = build_didl_lite(uri, title, content_type)
        logger.info(f"Setting AVTransport URI: {uri}")
        logger.debug(f"DIDL metadata:\n{metadata}")

        body = self._build_soap_body(
            "AVTransport",
            "SetAVTransportURI",
            {
                "InstanceID": 0,
                "CurrentURI": uri,
                "CurrentURIMetaData": metadata,
            },
        )

        result = self._send_soap(
            self.av_transport_url, "AVTransport", "SetAVTransportURI", body
        )
        if result:
            logger.info(f"SetAVTransportURI succeeded, response:\n{result.get('response', '')}")
        return result is not None

    def play(self, speed: str = "1") -> bool:
        if not self.av_transport_url:
            return False

        body = self._build_soap_body("AVTransport", "Play", {"InstanceID": 0, "Speed": speed})
        result = self._send_soap(self.av_transport_url, "AVTransport", "Play", body)
        return result is not None

    def pause(self) -> bool:
        if not self.av_transport_url:
            return False

        body = self._build_soap_body("AVTransport", "Pause", {"InstanceID": 0})
        result = self._send_soap(self.av_transport_url, "AVTransport", "Pause", body)
        return result is not None

    def stop(self) -> bool:
        if not self.av_transport_url:
            return False

        body = self._build_soap_body("AVTransport", "Stop", {"InstanceID": 0})
        result = self._send_soap(self.av_transport_url, "AVTransport", "Stop", body)
        return result is not None

    def seek(self, target: str, unit: str = "REL_TIME") -> bool:
        if not self.av_transport_url:
            return False

        body = self._build_soap_body(
            "AVTransport", "Seek", {"InstanceID": 0, "Unit": unit, "Target": target}
        )
        result = self._send_soap(self.av_transport_url, "AVTransport", "Seek", body)
        return result is not None

    def get_position_info(self) -> dict | None:
        if not self.av_transport_url:
            return None

        body = self._build_soap_body("AVTransport", "GetPositionInfo", {"InstanceID": 0})
        result = self._send_soap(self.av_transport_url, "AVTransport", "GetPositionInfo", body)
        if not result:
            return None

        resp_text = result.get("response", "")
        rel_time = self._parse_soap_response(resp_text, "RelTime")
        duration = self._parse_soap_response(resp_text, "Duration")
        return {"rel_time": rel_time, "duration": duration, "raw": resp_text}

    def get_transport_info(self) -> dict | None:
        if not self.av_transport_url:
            return None

        body = self._build_soap_body("AVTransport", "GetTransportInfo", {"InstanceID": 0})
        result = self._send_soap(self.av_transport_url, "AVTransport", "GetTransportInfo", body)
        if not result:
            return None

        resp_text = result.get("response", "")
        state = self._parse_soap_response(resp_text, "CurrentTransportState")
        return {"state": state, "raw": resp_text}

    def set_volume(self, volume: int) -> bool:
        if not self.rendering_control_url:
            return False

        body = self._build_soap_body(
            "RenderingControl",
            "SetVolume",
            {"InstanceID": 0, "Channel": "Master", "DesiredVolume": volume},
        )
        result = self._send_soap(
            self.rendering_control_url, "RenderingControl", "SetVolume", body
        )
        return result is not None

    def get_volume(self) -> int | None:
        if not self.rendering_control_url:
            return None

        body = self._build_soap_body(
            "RenderingControl", "GetVolume", {"InstanceID": 0, "Channel": "Master"}
        )
        result = self._send_soap(
            self.rendering_control_url, "RenderingControl", "GetVolume", body
        )
        if not result:
            return None

        resp_text = result.get("response", "")
        vol_str = self._parse_soap_response(resp_text, "CurrentVolume")
        if vol_str:
            try:
                return int(vol_str)
            except ValueError:
                pass
        return None

    def close(self):
        pass
