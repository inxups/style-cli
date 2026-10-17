import copy
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import os
import subprocess
import sys

from m3_probe_support import SourceBinding, PrivateBaseline, correlate_probe, private_write, sha


class M3EvidenceTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(); self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        for name in ('style', 'native', 'collector'):
            (self.root/name).write_text(name)
        self.binding = SourceBinding({key: self.root/name for key, name in [('style_binary','style'),('harness_binary','native'),('collector','collector')]})
        self.report = {'format':1, 'status':'observed', 'cleanup':{'complete':True}, 'targets':[{'harness':'codex','route':'codex-user','callback_observed':True,'event':'SessionStart','session_sha256':sha(b'session'),'runtime_sha256':self.binding.hashes['style_binary'],'harness_sha256':self.binding.hashes['harness_binary'],'observed_at':'2026-10-18T00:00:00Z'}]}
        self.native = [{'event':'SessionStart','source':'codex-user','completed':True,'started_ns':1792281599000000000,'ended_ns':1792281601000000000}]
        self.delivery = [{'body_sha256':sha(b'complete fixture body'),'complete_body':True,'request_occurrences':1}]

    def verify(self, **kw):
        return correlate_probe(kw.get('report', self.report), 'codex', 'codex-user', 'session', 'SessionStart', kw.get('native',self.native), kw.get('delivery',self.delivery), self.binding)

    def test_accepts_bound_native_evidence(self):
        self.assertTrue(self.verify()['cleanup_complete'])

    def test_rejects_each_source_and_binary_drift(self):
        for path in self.binding.files.values():
            old=path.read_bytes();path.write_bytes(old+b'drift')
            with self.assertRaisesRegex(ValueError,'source drift'): self.verify()
            path.write_bytes(old)

    def test_observer_copy_is_checked_and_bound(self):
        copy_path=self.root/'copied-observer';copy_path.write_bytes((self.root/'collector').read_bytes())
        self.binding.add_copy('observer_copy',copy_path,self.root/'collector')
        copy_path.write_text('changed copy')
        with self.assertRaisesRegex(ValueError,'source drift'):self.binding.verify()
        with self.assertRaisesRegex(ValueError,'copy drift'):self.binding.add_copy('second_copy',copy_path,self.root/'collector')

    def test_callback_or_ancestor_alone_does_not_prove_native_delivery(self):
        for native in ([],[{'event':'PreToolUse','source':'codex-user','completed':True}], [{'event':'SessionStart','source':'codex-plugin','completed':True}]):
            with self.subTest(native=native), self.assertRaisesRegex(ValueError,'native event missing'): self.verify(native=native)
        for delivery in ([],[dict(self.delivery[0], complete_body=False)], [dict(self.delivery[0],request_occurrences=0)]):
            with self.subTest(delivery=delivery), self.assertRaisesRegex(ValueError,'complete body absent'): self.verify(delivery=delivery)

    def test_rejects_wrong_session_binary_cleanup_and_status(self):
        for key,value in [('event','PreToolUse'),('session_sha256',sha(b'foreign')),('runtime_sha256',sha(b'other')),('harness_sha256',sha(b'other'))]:
            report=copy.deepcopy(self.report);report['targets'][0][key]=value
            with self.assertRaises(ValueError): self.verify(report=report)

    def test_rejects_unrelated_native_time_and_missing_observation_time(self):
        with self.assertRaisesRegex(ValueError,'native event missing'):self.verify(native=[dict(self.native[0],started_ns=1,ended_ns=2)])
        report=copy.deepcopy(self.report);del report['targets'][0]['observed_at']
        with self.assertRaisesRegex(ValueError,'time missing'):self.verify(report=report)
        for report in (dict(self.report,status='timed-out'),dict(self.report,cleanup={'complete':False})):
            with self.assertRaises(ValueError): self.verify(report=report)

    def test_private_baseline_roundtrip_and_secret_exclusion(self):
        existing=self.root/'settings.json';existing.write_bytes(b'{ "keep":true }\r\n');existing.chmod(0o640)
        baseline=PrivateBaseline(self.root,['settings.json','new-config'])
        backup=self.root/'baseline.private.json';baseline.save(backup)
        self.assertEqual(backup.stat().st_mode & 0o777, 0o600)
        existing.write_text('modified');(self.root/'new-config').write_text('created')
        with self.assertRaises(ValueError): baseline.verify()
        self.assertTrue(baseline.restore()['bytes_and_modes_restored'])
        for name in ('../outside','auth.json','.credentials.json','.claude.json'):
            with self.assertRaises(ValueError): PrivateBaseline(self.root,[name])

    def test_private_write_handles_short_write_and_rejects_symlink(self):
        writer=os.write
        with patch('m3_probe_support.os.write',side_effect=lambda fd,data: writer(fd,data[:3])):
            private_write(self.root/'private',b'full body despite short writes')
        self.assertEqual((self.root/'private').read_bytes(),b'full body despite short writes')
        (self.root/'link').symlink_to(self.root/'private')
        with self.assertRaises(OSError): private_write(self.root/'link',b'overwrite')

    def test_baseline_rejects_replaced_leaf_and_parent_symlink(self):
        (self.root/'nested').mkdir(); path=self.root/'nested/settings.json'; path.write_text('keep')
        baseline=PrivateBaseline(self.root,['nested/settings.json'])
        path.unlink();path.symlink_to(self.root/'style')
        with self.assertRaisesRegex(ValueError,'symlink'): baseline.restore()
        path.unlink();(self.root/'nested').rmdir();(self.root/'nested').symlink_to(self.root)
        with self.assertRaisesRegex(ValueError,'symlink'): baseline.restore()

    def test_independent_observer_preserves_complete_raw_payload(self):
        raw=' {"session_id":"fixture", "hook_event_name":"SessionStart", "prompt":"原始正文"}\n'.encode()
        destination=self.root/'native-events'
        env=os.environ.copy();env.update(PLUGIN_ROOT='/fixture/native-cache',CLAUDE_PLUGIN_ROOT='/fixture/native-cache',OBSERVER_SECRET='must-not-collect')
        result=subprocess.run([sys.executable,'-I',str(Path(__file__).with_name('m3_native_observer.py')),str(destination),'fixture-marker'],input=raw,capture_output=True,check=True,env=env)
        files=list(destination.glob('*.json'));self.assertEqual(len(files),1)
        self.assertEqual(files[0].read_bytes(),raw)
        self.assertEqual(files[0].stat().st_mode & 0o777,0o600)
        origin=files[0].with_suffix('.origin')
        self.assertEqual(json.loads(origin.read_bytes()),{'plugin_root':'/fixture/native-cache','claude_plugin_root':'/fixture/native-cache'})
        self.assertEqual(origin.stat().st_mode & 0o777,0o600)
        self.assertEqual(json.loads(result.stdout)['hookSpecificOutput']['additionalContext'],'fixture-marker')


if __name__=='__main__': unittest.main()
