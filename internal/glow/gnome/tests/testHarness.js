// Minimal test framework for GJS standalone testing
let _passed = 0;
let _failed = 0;

export function assert(condition, message) {
    if (!condition) {
        _failed++;
        printerr(`  FAIL: ${message}`);
    } else {
        _passed++;
        print(`  PASS: ${message}`);
    }
}

export function assertApprox(actual, expected, tolerance, message) {
    const diff = Math.abs(actual - expected);
    assert(diff < tolerance,
        `${message} (expected ~${expected}, got ${actual}, diff ${diff})`);
}

export function assertEqual(actual, expected, message) {
    assert(actual === expected,
        `${message} (expected ${JSON.stringify(expected)}, got ${JSON.stringify(actual)})`);
}

export function suite(name, fn) {
    print(`\n--- ${name} ---`);
    fn();
}

export function summary() {
    print(`\n=== ${_passed} passed, ${_failed} failed ===`);
    if (_failed > 0) {
        throw new Error(`${_failed} tests failed`);
    }
}
