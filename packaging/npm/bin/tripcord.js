#!/usr/bin/env node
'use strict';

// Runs the tripcord release binary with this process's arguments,
// stdio, and exit status. See lib/tripcord.js for how the binary is
// fetched and verified.

const { spawn } = require('node:child_process');
const { ensureBinary } = require('../lib/tripcord.js');

ensureBinary().then(
  (bin) => {
    const child = spawn(bin, process.argv.slice(2), { stdio: 'inherit', windowsHide: false });
    const forward = (signal) => {
      if (child.exitCode === null) child.kill(signal);
    };
    for (const signal of ['SIGINT', 'SIGTERM', 'SIGHUP']) process.on(signal, () => forward(signal));
    child.on('error', (err) => {
      process.stderr.write(`tripcord: cannot run ${bin}: ${err.message}\n`);
      process.exit(1);
    });
    child.on('exit', (code, signal) => {
      if (signal) {
        process.removeAllListeners(signal);
        process.kill(process.pid, signal);
        return;
      }
      process.exit(code === null ? 1 : code);
    });
  },
  (err) => {
    process.stderr.write(`${err.message}\n`);
    process.exit(1);
  },
);
