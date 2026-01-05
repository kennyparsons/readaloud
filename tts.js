const { EdgeTTS } = require('edge-tts-universal');
const fs = require('fs/promises');

async function main() {
    const args = process.argv.slice(2);
    const params = {};
    
    for (let i = 0; i < args.length; i++) {
        if (args[i].startsWith('--')) {
            const key = args[i].slice(2);
            params[key] = args[i + 1];
            i++;
        }
    }

    if (!params.text || !params.output) {
        console.error('Missing --text or --output');
        process.exit(1);
    }

    try {
        const tts = new EdgeTTS(params.text, params.voice || 'en-US-AriaNeural', {
            rate: params.rate || '+0%',
            volume: params.volume || '+0%',
            pitch: '+0Hz'
        });
        
        const result = await tts.synthesize();
        const buffer = Buffer.from(await result.audio.arrayBuffer());
        await fs.writeFile(params.output, buffer);
    } catch (err) {
        console.error(err);
        process.exit(1);
    }
}

main();
