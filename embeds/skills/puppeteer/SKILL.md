# Puppeteer Browser Automation Skill

## Overview
Puppeteer is a Node.js library that provides a high-level API to control Chrome/Chromium browsers. Use this skill for browser automation, web scraping, testing, and generating screenshots or PDFs from web pages.

## When to Use This Skill
- Scraping data from websites (especially JavaScript-heavy sites)
- Taking screenshots of web pages
- Generating PDFs from web content
- Automating form submissions
- Testing web applications
- Monitoring websites for changes
- Extracting structured data that requires browser rendering

## Prerequisites

### Installation
```bash
# Create and/or navigate to the directory for the project
mkdir -p /tmp/myproject
cd /tmp/

# Initialize npm project (if needed)
npm init -y

# Install Puppeteer
npm install puppeteer

# Verify installation
node -e "console.log(require('puppeteer'))"
```

### System Requirements
- Node.js 14+ installed

## Core Patterns

### Pattern 1: Basic Page Navigation & Scraping
```javascript
const puppeteer = require('puppeteer');

(async () => {
    // Launch browser
    const browser = await puppeteer.launch({
        headless: true,  // Run in background (no GUI)
        args: ['--no-sandbox', '--disable-setuid-sandbox']  // For server environments
    });
    
    // Open new page
    const page = await browser.newPage();
    
    // Navigate to URL
    await page.goto('https://example.com', {
        waitUntil: 'networkidle2'  // Wait until network is idle
    });
    
    // Extract data
    const data = await page.evaluate(() => {
        // This code runs in the browser context
        return {
            title: document.title,
            heading: document.querySelector('h1')?.textContent,
            links: Array.from(document.querySelectorAll('a')).map(a => ({
                text: a.textContent,
                href: a.href
            }))
        };
    });
    
    console.log(data);
    
    // Close browser
    await browser.close();
})();
```

### Pattern 2: Taking Screenshots
```javascript
const puppeteer = require('puppeteer');

(async () => {
    const browser = await puppeteer.launch({ headless: true });
    const page = await browser.newPage();
    
    // Set viewport size
    await page.setViewport({
        width: 1920,
        height: 1080,
        deviceScaleFactor: 1
    });
    
    await page.goto('https://example.com', { waitUntil: 'networkidle2' });
    
    // Full page screenshot
    await page.screenshot({
        path: '/tmp/myproject/screenshot-full.png',
        fullPage: true
    });
    
    // Specific element screenshot
    const element = await page.$('header');
    await element.screenshot({
        path: '/tmp/myproject/screenshot-element.png'
    });
    
    await browser.close();
})();
```

### Pattern 3: Generate PDF
```javascript
const puppeteer = require('puppeteer');

(async () => {
    const browser = await puppeteer.launch({ headless: true });
    const page = await browser.newPage();
    
    await page.goto('https://example.com', { waitUntil: 'networkidle2' });
    
    // Generate PDF
    await page.pdf({
        path: '/tmp/myproject/page.pdf',
        format: 'A4',
        printBackground: true,
        margin: {
            top: '20px',
            right: '20px',
            bottom: '20px',
            left: '20px'
        }
    });
    
    await browser.close();
})();
```

### Pattern 4: Form Automation
```javascript
const puppeteer = require('puppeteer');

(async () => {
    const browser = await puppeteer.launch({ headless: true });
    const page = await browser.newPage();
    
    await page.goto('https://example.com/form', { waitUntil: 'networkidle2' });
    
    // Fill out form
    await page.type('#username', 'myusername');
    await page.type('#password', 'mypassword');
    
    // Select dropdown
    await page.select('#country', 'US');
    
    // Check checkbox
    await page.click('#agree-terms');
    
    // Click submit button
    await page.click('button[type="submit"]');
    
    // Wait for navigation
    await page.waitForNavigation({ waitUntil: 'networkidle2' });
    
    // Get result
    const result = await page.evaluate(() => document.body.textContent);
    console.log(result);
    
    await browser.close();
})();
```

### Pattern 5: Data Extraction (Table Scraping)
```javascript
const puppeteer = require('puppeteer');

(async () => {
    const browser = await puppeteer.launch({ headless: true });
    const page = await browser.newPage();
    
    await page.goto('https://example.com/table', { waitUntil: 'networkidle2' });
    
    // Extract table data
    const tableData = await page.evaluate(() => {
        const rows = Array.from(document.querySelectorAll('table tr'));
        
        return rows.map(row => {
            const cells = Array.from(row.querySelectorAll('td, th'));
            return cells.map(cell => cell.textContent.trim());
        });
    });
    
    console.log(tableData);
    
    await browser.close();
})();
```

### Pattern 6: Waiting for Content
```javascript
const puppeteer = require('puppeteer');

(async () => {
    const browser = await puppeteer.launch({ headless: true });
    const page = await browser.newPage();
    
    await page.goto('https://example.com', { waitUntil: 'networkidle2' });
    
    // Wait for specific selector
    await page.waitForSelector('.dynamic-content', {
        visible: true,
        timeout: 5000
    });
    
    // Wait for function
    await page.waitForFunction(
        () => document.querySelector('.data-loaded')?.textContent !== 'Loading...',
        { timeout: 10000 }
    );
    
    // Extract after waiting
    const content = await page.$eval('.dynamic-content', el => el.textContent);
    
    await browser.close();
})();
```

### Pattern 7: Handling Multiple Pages
```javascript
const puppeteer = require('puppeteer');

(async () => {
    const browser = await puppeteer.launch({ headless: true });
    
    const urls = [
        'https://example.com/page1',
        'https://example.com/page2',
        'https://example.com/page3'
    ];
    
    const results = [];
    
    for (const url of urls) {
        const page = await browser.newPage();
        await page.goto(url, { waitUntil: 'networkidle2' });
        
        const data = await page.evaluate(() => ({
            title: document.title,
            url: window.location.href
        }));
        
        results.push(data);
        await page.close();
    }
    
    console.log(results);
    await browser.close();
})();
```

## Best Practices

### 1. Resource Management
```javascript
// ALWAYS close browser when done
try {
    const browser = await puppeteer.launch();
    const page = await browser.newPage();
    // ... do work ...
} catch (error) {
    console.error('Error:', error);
} finally {
    await browser.close();  // Ensure cleanup
}
```

### 2. Error Handling
```javascript
const puppeteer = require('puppeteer');

(async () => {
    let browser;
    try {
        browser = await puppeteer.launch({ headless: true });
        const page = await browser.newPage();
        
        // Set timeout
        page.setDefaultTimeout(30000);
        
        await page.goto('https://example.com', {
            waitUntil: 'networkidle2',
            timeout: 30000
        });
        
        // Check if element exists before interacting
        const elementExists = await page.$('.my-selector') !== null;
        if (!elementExists) {
            throw new Error('Element not found');
        }
        
    } catch (error) {
        console.error('Puppeteer error:', error.message);
        // Handle specific errors
        if (error.message.includes('timeout')) {
            console.error('Page took too long to load');
        }
    } finally {
        if (browser) await browser.close();
    }
})();
```

### 3. Performance Optimization
```javascript
const browser = await puppeteer.launch({
    headless: true,
    args: [
        '--no-sandbox',
        '--disable-setuid-sandbox',
        '--disable-dev-shm-usage',  // Overcome limited resource problems
        '--disable-gpu',
        '--disable-web-security',
        '--disable-features=IsolateOrigins,site-per-process'
    ]
});

// Block unnecessary resources
await page.setRequestInterception(true);
page.on('request', (request) => {
    const resourceType = request.resourceType();
    if (['image', 'stylesheet', 'font'].includes(resourceType)) {
        request.abort();  // Block images, CSS, fonts for faster loading
    } else {
        request.continue();
    }
});
```

### 4. User Agent & Headers
```javascript
await page.setUserAgent(
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36'
);

await page.setExtraHTTPHeaders({
    'Accept-Language': 'en-US,en;q=0.9'
});
```

## Common Tasks

### Task: Scrape Product Information
```javascript
const puppeteer = require('puppeteer');

(async () => {
    const browser = await puppeteer.launch({ headless: true });
    const page = await browser.newPage();
    
    await page.goto('https://example-shop.com/products', {
        waitUntil: 'networkidle2'
    });
    
    const products = await page.evaluate(() => {
        return Array.from(document.querySelectorAll('.product')).map(product => ({
            name: product.querySelector('.product-name')?.textContent.trim(),
            price: product.querySelector('.product-price')?.textContent.trim(),
            image: product.querySelector('img')?.src,
            link: product.querySelector('a')?.href
        }));
    });
    
    console.log(JSON.stringify(products, null, 2));
    await browser.close();
})();
```

### Task: Monitor Page Changes
```javascript
const puppeteer = require('puppeteer');
const fs = require('fs');

(async () => {
    const browser = await puppeteer.launch({ headless: true });
    const page = await browser.newPage();
    
    await page.goto('https://example.com/status', { waitUntil: 'networkidle2' });
    
    const currentContent = await page.evaluate(() => document.body.textContent);
    
    // Compare with previous content
    const previousContent = fs.existsSync('/tmp/myproject/previous.txt')
        ? fs.readFileSync('/tmp/myproject/previous.txt', 'utf-8')
        : '';
    
    if (currentContent !== previousContent) {
        console.log('Page has changed!');
        fs.writeFileSync('/tmp/myproject/previous.txt', currentContent);
    } else {
        console.log('No changes detected');
    }
    
    await browser.close();
})();
```

### Task: Login and Scrape Authenticated Content
```javascript
const puppeteer = require('puppeteer');

(async () => {
    const browser = await puppeteer.launch({ headless: true });
    const page = await browser.newPage();
    
    // Go to login page
    await page.goto('https://example.com/login', { waitUntil: 'networkidle2' });
    
    // Fill login form
    await page.type('#username', 'myuser');
    await page.type('#password', 'mypass');
    await page.click('button[type="submit"]');
    
    // Wait for navigation after login
    await page.waitForNavigation({ waitUntil: 'networkidle2' });
    
    // Now access protected page
    await page.goto('https://example.com/dashboard', { waitUntil: 'networkidle2' });
    
    const data = await page.evaluate(() => ({
        accountInfo: document.querySelector('.account-info')?.textContent
    }));
    
    console.log(data);
    await browser.close();
})();
```

## Project Structure
```bash
# Create Puppeteer project
mkdir -p /tmp/myproject/web-scraping/{scripts,data,outputs}

# Initialize npm
cd /tmp/myproject/web-scraping/
npm init -y
npm install puppeteer

# Create script
# (Write your scraping script to scripts/scraper.js)

# Run
node scripts/scraper.js
```

## Saving Results
```javascript
const fs = require('fs');

// Save JSON
fs.writeFileSync(
    '/tmp/myproject/web-scraping/data/results.json',
    JSON.stringify(data, null, 2)
);

// Save CSV
const csv = data.map(row => Object.values(row).join(',')).join('\n');
fs.writeFileSync('/tmp/myproject/web-scraping/data/results.csv', csv);

// Copy to outputs for user
fs.copyFileSync(
    '/tmp/myproject/web-scraping/data/results.json',
    '/tmp/myproject/outputs/results.json'
);
```

## Troubleshooting

### Issue: "Browser not found"
```bash
# Solution: Explicitly set executable path
const browser = await puppeteer.launch({
    executablePath: '/usr/bin/chromium-browser',
    headless: true
});
```

### Issue: Timeout errors
```javascript
// Increase timeout
await page.goto(url, {
    waitUntil: 'networkidle2',
    timeout: 60000  // 60 seconds
});
```

### Issue: Element not found
```javascript
// Check if element exists
const elementExists = await page.$('.selector') !== null;
if (elementExists) {
    await page.click('.selector');
} else {
    console.log('Element not found, skipping...');
}
```

### Issue: Memory leaks
```javascript
// Always close pages and browser
await page.close();
await browser.close();

// Use page pooling for multiple operations
```

## Security Considerations

- ⚠️ **Respect rate limits** - add delays between requests
- ⚠️ **Don't expose credentials** in code - use environment variables
- ⚠️ **Sanitize scraped data** before using it

## Rate Limiting
```javascript
// Add delays between requests
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));

for (const url of urls) {
    await page.goto(url);
    // ... scrape ...
    await delay(2000);  // Wait 2 seconds between requests
}
```

## References

- Official Docs: https://pptr.dev/
- API Reference: https://pptr.dev/api
- Examples: https://github.com/puppeteer/puppeteer/tree/main/examples
- Troubleshooting: https://pptr.dev/troubleshooting
