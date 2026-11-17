<?php
// Runs cases through Banko's real routes. Usage, from a Banko checkout with
// vendor/ installed:
//   php /path/to/banko-harness.php < cases.jsonl > banko.jsonl
// Each input line: {"id": "...", "now": "<RFC3339>", "path": "/api/utils/...?..."}
// Each output line: {"id": "...", "status": 200, "body": "..."}

use Illuminate\Contracts\Http\Kernel;
use Illuminate\Http\Request;
use Illuminate\Support\Carbon;

require getcwd() . '/vendor/autoload.php';
$app = require getcwd() . '/bootstrap/app.php';
$kernel = $app->make(Kernel::class);
// Banko's api routes are throttled at 60/min; lift that so a sweep can run.
$kernel->bootstrap();
\Illuminate\Support\Facades\RateLimiter::for('api', fn () => \Illuminate\Cache\RateLimiting\Limit::none());

while (($line = fgets(STDIN)) !== false) {
    $c = json_decode($line, true);
    if (!$c) continue;
    Carbon::setTestNow(Carbon::parse($c['now']));
    $req = Request::create($c['path'], 'GET', [], [], [], ['HTTP_ACCEPT' => 'application/json']);
    try {
        $res = $kernel->handle($req);
        $out = ['id' => $c['id'], 'status' => $res->getStatusCode(), 'body' => (string) $res->getContent()];
        $kernel->terminate($req, $res);
    } catch (\Throwable $e) {
        $out = ['id' => $c['id'], 'status' => -1, 'body' => get_class($e) . ': ' . $e->getMessage()];
    }
    echo json_encode($out, JSON_UNESCAPED_UNICODE | JSON_UNESCAPED_SLASHES), "\n";
}
