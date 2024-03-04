<?php

use Nyholm\Psr7\Response;
use Spiral\Goridge;
use Spiral\RoadRunner;

ini_set('display_errors', 'stderr');
require __DIR__ . "/vendor/autoload.php";

$worker = new RoadRunner\Worker(new Goridge\StreamRelay(STDIN, STDOUT));
$psr17Factory = new \Nyholm\Psr7\Factory\Psr17Factory();
$psr7 = new RoadRunner\Http\PSR7Worker($worker, $psr17Factory, $psr17Factory, $psr17Factory);

while ($req = $psr7->waitRequest()) {
    try {
        switch ($req->getUri()->getPath()) {
            case "/local-file":
                $resp = new Response(200, ["X-Sendremotefile" => "/../sample/2k24.mp4"]);
                break;

            case "/remote-file":
                $resp = new Response(200, ["X-Sendremotefile" => "http://127.0.0.1:18953/file", "Content-Disposition" => "attachment; filename=composer.json"]);
                break;

            case "/remote-file-not-found":
                $resp = new Response(200, ["X-Sendremotefile" => "http://127.0.0.1:18953/file-missing"]);
                break;

            case "/remote-file-timeout":
                $resp = new Response(200, ["X-Sendremotefile" => "http://127.0.0.1:18953/file-timeout"]);
                break;
            
            case "/file":
                $resp = new Response(200, ["Content-Type" => "text/plain"], $psr17Factory->createStreamFromFile(__DIR__ . "/composer.json"));
                break;
            
            case "/file-timeout":
                usleep(5_500_000);
                $resp = new Response(200, ["Content-Type" => "text/plain"], $psr17Factory->createStreamFromFile(__DIR__ . "/composer.json"));
                break;
            
            default:
                $resp = new Response(404);
                break;
        }

        $psr7->respond($resp);
        unset($resp);
    } catch (\Throwable $e) {
        $psr7->getWorker()->error((string)$e);
    }
}
