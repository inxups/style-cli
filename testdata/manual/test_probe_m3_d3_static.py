import io
import json
import unittest

import probe_m3_d3_static as probe


class CaptureTest(unittest.TestCase):
    def test_records_only_marker_fields_and_replays_identical_body(self):
        class Handler:
            def do_POST(self):
                self.forwarded = self.rfile.read(int(self.headers["Content-Length"]))

        class Server:
            RequestHandlerClass = Handler

        class Stub:
            server = Server()

        stub = Stub()
        rows = probe.capture(stub)
        body = json.dumps({"input": [probe.MARKERS["user"], "private-request-body"],
                           "instructions": probe.MARKERS["project"]}).encode()
        handler = object.__new__(stub.server.RequestHandlerClass)
        handler.rfile = io.BytesIO(body)
        handler.headers = {"Content-Length": str(len(body))}
        handler.path = "/v1/responses?private-query"
        handler.do_POST()
        self.assertEqual(handler.forwarded, body)
        self.assertEqual(rows, [{"path": "/v1/responses", "markers": {
            "user": ["input"], "project": ["instructions"], "memory": []}}])
        self.assertNotIn("private", json.dumps(rows))


if __name__ == "__main__":
    unittest.main()
