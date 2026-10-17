#!/usr/bin/env python3
"""Regenerate public exact text references with tiktoken==0.12.0.

Optional acceptance tool; no Python dependency is used by the production CLI.
Warm TIKTOKEN_CACHE_DIR permits a network-disabled regeneration.
"""
import argparse
import hashlib
import importlib.metadata
import json
from pathlib import Path
import random
import struct


def vocabulary_sha256(encoding):
    digest = hashlib.sha256()
    for value, rank in sorted(encoding._mergeable_ranks.items(), key=lambda pair: pair[1]):
        digest.update(struct.pack('>QQ', rank, len(value)))
        digest.update(value)
    return digest.hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, default=Path('testdata/count/exact-text-v1.json'))
    args = parser.parse_args()
    import tiktoken
    assert importlib.metadata.version('tiktoken') == '0.12.0'
    corpus = Path('testdata/manual/token-calibration-40.json')
    cases = [(item['id'], item['text']) for item in json.loads(corpus.read_text())['rules']]
    cases += [(str(i), text) for i, text in enumerate([
        '', ' ', '\r\n\t', 'hello world', '你好，世界！', '👩🏽\u200d💻🙂🇯🇵',
        'e\u0301 café Å Å', '<|endoftext|> <|endofprompt|> <|fim_prefix|>',
        "I'm WE'RE They've can't It'll", 'line\n\n二行\r\n',
        '\x00\x01\x1f\x7f\u2028\u2029', 'א ב العربية हिन्दी ไทย 한국어 カタカナ',
        '1234567890 0.5 -42 3e10', '.*?[]{}()\\/!=#++==', 'a' * 4096,
        ' ' * 4096, ' \n\t' * 256,
    ])]
    rng = random.Random(53302156)
    alphabet = list('AaÉéßİıΩЖ中日本かな한글अعאב01239 _-.,\n\r\t') + ['🙂', '\u0301', '\u200d', '\u2028', '\x00']
    cases += [('seeded-' + str(i), ''.join(rng.choice(alphabet) for _ in range(rng.randrange(1, 180)))) for i in range(80)]
    encodings = {name: tiktoken.get_encoding(name) for name in ('o200k_base', 'cl100k_base')}
    reference = dict(format=1, generator='scripts/generate_m3_exact_golden.py',
                     tiktoken_version='0.12.0', special_tokens='ordinary text; encode_ordinary',
                     source_corpus_sha256=hashlib.sha256(corpus.read_bytes()).hexdigest(),
                     vocabularies={name: dict(tokens=len(enc._mergeable_ranks), sha256=vocabulary_sha256(enc))
                                   for name, enc in encodings.items()},
                     cases=[dict(id=name, text=text, sha256=hashlib.sha256(text.encode()).hexdigest(),
                                 tokens={key: len(enc.encode_ordinary(text)) for key, enc in encodings.items()})
                            for name, text in cases])
    args.output.write_text(json.dumps(reference, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(dict(cases=len(cases), encodings=len(encodings))))


if __name__ == '__main__':
    main()
