const express = require('express');
const app = express();

// CPU-intensive function
function calculatePrimes(count) {
    const primes = [];
    let num = 2;

    while (primes.length < count) {
        if (isPrime(num)) {
            primes.push(num);
        }
        num++;
    }
    return primes;
}

function isPrime(num) {
    for (let i = 2; i <= Math.sqrt(num); i++) {
        if (num % i === 0) return false;
    }
    return num > 1;
}

// Normal endpoint
app.get('/', (req, res) => {
    res.json({ message: 'Service is running' });
});

// CPU-intensive endpoint
app.get('/cpu', (req, res) => {
    const primes = calculatePrimes(5000);
    res.json({ primes: primes.length });
});

app.listen('127.0.0.1:' + process.env.APP_PORT || 3030, () => {
    console.log('Server running on port 3000');
});