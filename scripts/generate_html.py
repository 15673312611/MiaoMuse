import argparse
import base64
import json
import re
import sys
import urllib.request
from pathlib import Path

from PIL import Image


ROOT = Path(__file__).resolve().parents[1]
DEFAULT_SCREENSHOTS = [
    "screenshots/03-creation-dashboard.png",
    "screenshots/07-original-script-steps.png",
    "screenshots/08-original-story-settings-form.png",
    "screenshots/15-coverage-evaluation-form.png",
    "screenshots/17-hot-lapian-grid.png",
    "screenshots/22-wallet-recharge-modal.png",
    "screenshots/24-community-courses-full.png",
    "screenshots/28-settings-profile.png",
]

PROMPT = (
    "帮我一比一复刻图中网站，用html生成，保留全部细节，要完全一模一样，直接返回完整html给我。\n"
    "只返回单个完整HTML，CSS和JS都内联，不要解释，不要Markdown，不要JSON。\n"
    "把截图当成像素级设计稿：还原元素位置、尺寸、间距、字体大小、颜色、圆角、阴影、弹窗遮罩、滚动区域、按钮状态和输入框状态。\n"
    "不要重新设计，不要脑补不存在的模块，不要把页面改成营销页。"
)


def parse_api_config(path: Path) -> dict:
    text = path.read_text(encoding="utf-8")
    url_match = re.search(r"https?://[^\s]+", text)
    model_match = re.search(r"model\s*[:：]\s*([^\s]+)", text, re.I)
    key_match = re.search(r"(sk-[A-Za-z0-9_\-]+|[A-Za-z0-9_\-]{32,})", text)
    lines = [line.strip() for line in text.splitlines() if line.strip()]
    if not model_match and len(lines) >= 2:
        model_match = re.match(r"(.+)", lines[1])
    if not key_match and len(lines) >= 3:
        key_match = re.match(r"(.+)", lines[2])
    if not url_match or not model_match or not key_match:
        raise ValueError("API config file must contain API base URL, model, and API key. Copy api.example.txt to local api.txt or pass --api.")
    return {
        "base_url": url_match.group(0).rstrip("/"),
        "model": model_match.group(1).strip(),
        "api_key": key_match.group(1).strip(),
    }


def image_part(path: Path) -> tuple[dict, tuple[int, int]]:
    with Image.open(path) as img:
        size = img.size
    mime = "image/png"
    payload = base64.b64encode(path.read_bytes()).decode("ascii")
    return {
        "type": "image_url",
        "image_url": {"url": f"data:{mime};base64,{payload}"},
    }, size


def extract_content(data: dict) -> str:
    if data.get("object") == "response":
        chunks = []
        for item in data.get("output", []):
            for part in item.get("content", []):
                if part.get("type") == "output_text":
                    chunks.append(part.get("text", ""))
        return "".join(chunks) or json.dumps(data, ensure_ascii=False, indent=2)
    choices = data.get("choices") or []
    if not choices:
        return ""
    message = choices[0].get("message") or {}
    content = message.get("content", "")
    if isinstance(content, str):
        return content
    if isinstance(content, list):
        return "".join(part.get("text", "") for part in content if isinstance(part, dict))
    return str(content)


def parse_response(raw: str) -> dict:
    if raw.lstrip().startswith("data:"):
        chunks = []
        for line in raw.splitlines():
            line = line.strip()
            if not line.startswith("data:"):
                continue
            payload = line[5:].strip()
            if not payload or payload == "[DONE]":
                continue
            try:
                event = json.loads(payload)
            except json.JSONDecodeError:
                continue
            for choice in event.get("choices", []):
                delta = choice.get("delta") or {}
                if isinstance(delta.get("content"), str):
                    chunks.append(delta["content"])
                message = choice.get("message") or {}
                if isinstance(message.get("content"), str):
                    chunks.append(message["content"])
            for item in event.get("output", []):
                for part in item.get("content", []):
                    text = part.get("text") or part.get("content")
                    if isinstance(text, str):
                        chunks.append(text)
        if chunks:
            return {"choices": [{"message": {"content": "".join(chunks)}}]}
        return {"choices": []}
    return json.loads(raw)


def main() -> int:
    parser = argparse.ArgumentParser(description="Generate a one-file HTML reference from StoryPlay screenshots.")
    parser.add_argument("--api", default=str(ROOT / "api.example.txt"), help="Path to local API config file")
    parser.add_argument("--out", default=str(ROOT / "docs" / "generated-reference.html"), help="Output HTML path")
    parser.add_argument("--screenshot", action="append", help="Screenshot path. Can be repeated.")
    parser.add_argument("--viewport", default="", help="CSS viewport size, e.g. 1605x1125. Defaults to each screenshot pixel size.")
    parser.add_argument("--note", default="", help="Extra page/interaction notes for this generation.")
    parser.add_argument("--max-tokens", type=int, default=12000)
    parser.add_argument("--max-width", type=int, default=0, help="Deprecated; images are sent uncompressed.")
    parser.add_argument("--max-height", type=int, default=0, help="Deprecated; images are sent uncompressed.")
    parser.add_argument("--quality", type=int, default=100, help="Deprecated; images are sent uncompressed.")
    parser.add_argument("--mode", choices=["responses", "chat"], default="responses")
    args = parser.parse_args()

    config = parse_api_config(Path(args.api))
    screenshot_paths = args.screenshot or DEFAULT_SCREENSHOTS
    images = []
    image_notes = []
    viewport = args.viewport.strip()
    for item in screenshot_paths:
        path = (ROOT / item).resolve() if not Path(item).is_absolute() else Path(item)
        if not path.exists():
            raise FileNotFoundError(path)
        part, (width, height) = image_part(path)
        images.append(part)
        css_size = viewport or f"{width}x{height}"
        image_notes.append(
            f"{path.name}: PNG物理像素 {width}x{height}px；对应CSS视口 {css_size}px；请按该CSS视口生成根容器、body和主要画布。"
        )

    prompt = PROMPT + "\n" + "\n".join(image_notes)
    if args.note.strip():
        prompt += "\n页面说明：" + args.note.strip()

    if args.mode == "responses":
        content = [{"type": "input_text", "text": prompt}]
        for image in images:
            content.append({"type": "input_image", "image_url": image["image_url"]["url"]})
        endpoint = f"{config['base_url']}/v1/responses"
        payload = {
            "model": config["model"],
            "input": [{"role": "user", "content": content}],
            "max_output_tokens": args.max_tokens,
        }
    else:
        endpoint = f"{config['base_url']}/v1/chat/completions"
        payload = {
            "model": config["model"],
            "messages": [
                {
                    "role": "user",
                    "content": [{"type": "text", "text": prompt}, *images],
                }
            ],
            "max_tokens": args.max_tokens,
            "stream": False,
        }
    req = urllib.request.Request(
        endpoint,
        data=json.dumps(payload).encode("utf-8"),
        headers={
            "Authorization": f"Bearer {config['api_key']}",
            "Content-Type": "application/json",
        },
        method="POST",
    )

    try:
        with urllib.request.urlopen(req, timeout=240) as resp:
            raw = resp.read().decode("utf-8", errors="replace")
            if not raw.strip():
                print("Generation failed: empty response body", file=sys.stderr)
                return 1
            try:
                data = parse_response(raw)
            except json.JSONDecodeError as exc:
                preview = raw[:1200].replace(config["api_key"], "***")
                print(f"Generation failed: response is not JSON: {exc}", file=sys.stderr)
                print(preview, file=sys.stderr)
                return 1
    except Exception as exc:
        print(f"Generation failed: {exc}", file=sys.stderr)
        return 1

    content = extract_content(data).strip()
    content = re.sub(r"^```(?:html)?\s*|\s*```$", "", content, flags=re.I | re.S).strip()
    if "<html" not in content.lower() and "<!doctype html" not in content.lower():
        print("Generation failed: model returned no complete HTML", file=sys.stderr)
        preview = content[:1200].replace(config["api_key"], "***")
        if preview:
            print(preview, file=sys.stderr)
        return 1
    out_path = Path(args.out)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(content, encoding="utf-8")
    print(f"Generated HTML saved to {out_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
