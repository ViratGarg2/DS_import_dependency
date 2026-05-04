import re


READ_OK_RE = re.compile(r"Successfully read \d+ bytes:\s*\n(.*)", re.DOTALL)


def extract_last_read_payload(output: str) -> str:
    matches = READ_OK_RE.findall(output)
    if not matches:
        raise AssertionError(f"No successful read payload found.\nOutput:\n{output}")
    payload = matches[-1]
    payload = payload.split("\ngfs>")[0]
    return payload.rstrip("\n")
