import requests
import json
import random
import time
import os

# Configuration
BASE_URL = "http://127.0.0.1:9090/api/v1"
INVOKE_URL = f"{BASE_URL}/queue/default/enqueue_batch"
PROMPTS_FILE = "prompts.txt"
OUTPUT_DIR = "downloaded_images"

def load_prompts(filepath):
    """Reads prompts from a file, removing empty lines."""
    with open(filepath, 'r', encoding='utf-8') as f:
        return [line.strip() for line in f if line.strip()]

def get_graph():
    """Returns the static workflow graph."""
    return {
        "id": "sdxl_graph:WUxxTVrURx",
        "nodes": {
            "sdxl_model_loader:mfrHcrrv2F": {
                "type": "sdxl_model_loader",
                "id": "sdxl_model_loader:mfrHcrrv2F",
                "model": {
                    "key": "145c0b26-5a9d-4761-a26f-ceb49d7eec63",
                    "hash": "blake3:a7383e54f2e4570678b0f18545b2eb8fd95325da76ccba8467dbdbd481cf6b99",
                    "path": "D:/sdmodels/wai/waiIllustriousSDXL_v140.safetensors",
                    "file_size": 6938040682,
                    "name": "waiIllustriousSDXL_v140",
                    "source": "D:/sdmodels/wai/waiIllustriousSDXL_v140.safetensors",
                    "source_type": "path",
                    "type": "main",
                    "base": "sdxl"
                },
                "is_intermediate": True,
                "use_cache": True
            },
            "positive_prompt:35IPyJEmEK": {
                "id": "positive_prompt:35IPyJEmEK",
                "type": "string",
                "is_intermediate": True,
                "use_cache": True
            },
            "pos_cond:tC1gW5nSAS": {
                "type": "sdxl_compel_prompt",
                "id": "pos_cond:tC1gW5nSAS",
                "is_intermediate": True,
                "use_cache": True
            },
            "pos_cond_collect:mW8Ms5alPt": {
                "type": "collect",
                "id": "pos_cond_collect:mW8Ms5alPt",
                "is_intermediate": True,
                "use_cache": True
            },
            "neg_cond:Ng2PTiZqo2": {
                "type": "sdxl_compel_prompt",
                "id": "neg_cond:Ng2PTiZqo2",
                "prompt": "<lazydn>, <lazyhand>, 3d",
                "style": "<lazydn>, <lazyhand>, 3d",
                "is_intermediate": True,
                "use_cache": True
            },
            "neg_cond_collect:ICF5OfpEb4": {
                "type": "collect",
                "id": "neg_cond_collect:ICF5OfpEb4",
                "is_intermediate": True,
                "use_cache": True
            },
            "seed:DVz4FhMt5E": {
                "id": "seed:DVz4FhMt5E",
                "type": "integer",
                "is_intermediate": True,
                "use_cache": True
            },
            "noise:sBKTrGAlZD": {
                "type": "noise",
                "id": "noise:sBKTrGAlZD",
                "use_cpu": True,
                "is_intermediate": True,
                "use_cache": True,
                "width": 888,
                "height": 1184
            },
            "denoise_latents:rk0Tpd5g40": {
                "type": "denoise_latents",
                "id": "denoise_latents:rk0Tpd5g40",
                "cfg_scale": 7,
                "cfg_rescale_multiplier": 0,
                "scheduler": "euler_a",
                "steps": 20,
                "is_intermediate": True,
                "use_cache": True,
                "denoising_start": 0,
                "denoising_end": 1
            },
            "core_metadata:aznHO0tknZ": {
                "id": "core_metadata:aznHO0tknZ",
                "type": "core_metadata",
                "is_intermediate": True,
                "use_cache": True,
                "cfg_scale": 7,
                "cfg_rescale_multiplier": 0,
                "model": {
                    "key": "145c0b26-5a9d-4761-a26f-ceb49d7eec63",
                    "hash": "blake3:a7383e54f2e4570678b0f18545b2eb8fd95325da76ccba8467dbdbd481cf6b99",
                    "name": "waiIllustriousSDXL_v140",
                    "base": "sdxl",
                    "type": "main"
                },
                "steps": 20,
                "rand_device": "cpu",
                "scheduler": "euler_a",
                "negative_prompt": "<lazydn>, <lazyhand>, 3d",
                "width": 888,
                "height": 1184,
                "generation_mode": "sdxl_txt2img",
                "loras": [
                    {
                        "model": {
                            "key": "44309e2f-22d0-43bb-a5bb-fbb487e78ff0",
                            "hash": "blake3:b760ce45dea27ce5bf6a087474f565362cd78f23f54f48a4938bce07579ddde4",
                            "name": "S1_Dramatic_Lighting_v3",
                            "base": "sdxl",
                            "type": "lora"
                        },
                        "weight": 1.5
                    },
                    {
                        "model": {
                            "key": "9b7bfa9d-1de2-40d4-868c-71f61fa98eaa",
                            "hash": "blake3:b10f3ccf308a54cf2827a020277058f97b5e919cee99f28e9cc9f7cf96234e8e",
                            "name": "Marin_Kitagawa_S1Arisa_Izayoi_cosplay_My_Dress-Up_Darling",
                            "base": "sdxl",
                            "type": "lora"
                        },
                        "weight": 0.7
                    }
                ]
            },
            "lora_collector:nwp7q8vgVT": {"id": "lora_collector:nwp7q8vgVT", "type": "collect", "is_intermediate": True, "use_cache": True},
            "sdxl_lora_collection_loader:SYWsqbzskD": {"type": "sdxl_lora_collection_loader", "id": "sdxl_lora_collection_loader:SYWsqbzskD", "is_intermediate": True, "use_cache": True},
            "lora_selector:XadfYa8qq9": {
                "type": "lora_selector",
                "id": "lora_selector:XadfYa8qq9",
                "lora": {"key": "44309e2f-22d0-43bb-a5bb-fbb487e78ff0", "hash": "blake3:b760ce45dea27ce5bf6a087474f565362cd78f23f54f48a4938bce07579ddde4", "name": "S1_Dramatic_Lighting_v3", "base": "sdxl", "type": "lora"},
                "weight": 1.5,
                "is_intermediate": True,
                "use_cache": True
            },
            "lora_selector:jqvcs1uQWA": {
                "type": "lora_selector",
                "id": "lora_selector:jqvcs1uQWA",
                "lora": {"key": "9b7bfa9d-1de2-40d4-868c-71f61fa98eaa", "hash": "blake3:b10f3ccf308a54cf2827a020277058f97b5e919cee99f28e9cc9f7cf96234e8e", "name": "Marin_Kitagawa_S1Arisa_Izayoi_cosplay_My_Dress-Up_Darling", "base": "sdxl", "type": "lora"},
                "weight": 0.7,
                "is_intermediate": True,
                "use_cache": True
            },
            "canvas_output:seBPJ6NLJ2": {"type": "l2i", "id": "canvas_output:seBPJ6NLJ2", "fp32": True, "is_intermediate": False, "use_cache": False}
        },
        "edges": [
            {"source": {"node_id": "positive_prompt:35IPyJEmEK", "field": "value"}, "destination": {"node_id": "pos_cond:tC1gW5nSAS", "field": "prompt"}},
            {"source": {"node_id": "positive_prompt:35IPyJEmEK", "field": "value"}, "destination": {"node_id": "pos_cond:tC1gW5nSAS", "field": "style"}},
            {"source": {"node_id": "pos_cond:tC1gW5nSAS", "field": "conditioning"}, "destination": {"node_id": "pos_cond_collect:mW8Ms5alPt", "field": "item"}},
            {"source": {"node_id": "pos_cond_collect:mW8Ms5alPt", "field": "collection"}, "destination": {"node_id": "denoise_latents:rk0Tpd5g40", "field": "positive_conditioning"}},
            {"source": {"node_id": "neg_cond:Ng2PTiZqo2", "field": "conditioning"}, "destination": {"node_id": "neg_cond_collect:ICF5OfpEb4", "field": "item"}},
            {"source": {"node_id": "neg_cond_collect:ICF5OfpEb4", "field": "collection"}, "destination": {"node_id": "denoise_latents:rk0Tpd5g40", "field": "negative_conditioning"}},
            {"source": {"node_id": "seed:DVz4FhMt5E", "field": "value"}, "destination": {"node_id": "noise:sBKTrGAlZD", "field": "seed"}},
            {"source": {"node_id": "noise:sBKTrGAlZD", "field": "noise"}, "destination": {"node_id": "denoise_latents:rk0Tpd5g40", "field": "noise"}},
            {"source": {"node_id": "denoise_latents:rk0Tpd5g40", "field": "latents"}, "destination": {"node_id": "canvas_output:seBPJ6NLJ2", "field": "latents"}},
            {"source": {"node_id": "seed:DVz4FhMt5E", "field": "value"}, "destination": {"node_id": "core_metadata:aznHO0tknZ", "field": "seed"}},
            {"source": {"node_id": "positive_prompt:35IPyJEmEK", "field": "value"}, "destination": {"node_id": "core_metadata:aznHO0tknZ", "field": "positive_prompt"}},
            {"source": {"node_id": "lora_collector:nwp7q8vgVT", "field": "collection"}, "destination": {"node_id": "sdxl_lora_collection_loader:SYWsqbzskD", "field": "loras"}},
            {"source": {"node_id": "sdxl_model_loader:mfrHcrrv2F", "field": "unet"}, "destination": {"node_id": "sdxl_lora_collection_loader:SYWsqbzskD", "field": "unet"}},
            {"source": {"node_id": "sdxl_model_loader:mfrHcrrv2F", "field": "clip"}, "destination": {"node_id": "sdxl_lora_collection_loader:SYWsqbzskD", "field": "clip"}},
            {"source": {"node_id": "sdxl_model_loader:mfrHcrrv2F", "field": "clip2"}, "destination": {"node_id": "sdxl_lora_collection_loader:SYWsqbzskD", "field": "clip2"}},
            {"source": {"node_id": "sdxl_lora_collection_loader:SYWsqbzskD", "field": "unet"}, "destination": {"node_id": "denoise_latents:rk0Tpd5g40", "field": "unet"}},
            {"source": {"node_id": "sdxl_lora_collection_loader:SYWsqbzskD", "field": "clip"}, "destination": {"node_id": "pos_cond:tC1gW5nSAS", "field": "clip"}},
            {"source": {"node_id": "sdxl_lora_collection_loader:SYWsqbzskD", "field": "clip"}, "destination": {"node_id": "neg_cond:Ng2PTiZqo2", "field": "clip"}},
            {"source": {"node_id": "sdxl_lora_collection_loader:SYWsqbzskD", "field": "clip2"}, "destination": {"node_id": "pos_cond:tC1gW5nSAS", "field": "clip2"}},
            {"source": {"node_id": "sdxl_lora_collection_loader:SYWsqbzskD", "field": "clip2"}, "destination": {"node_id": "neg_cond:Ng2PTiZqo2", "field": "clip2"}},
            {"source": {"node_id": "lora_selector:XadfYa8qq9", "field": "lora"}, "destination": {"node_id": "lora_collector:nwp7q8vgVT", "field": "item"}},
            {"source": {"node_id": "lora_selector:jqvcs1uQWA", "field": "lora"}, "destination": {"node_id": "lora_collector:nwp7q8vgVT", "field": "item"}},
            {"source": {"node_id": "sdxl_model_loader:mfrHcrrv2F", "field": "vae"}, "destination": {"node_id": "canvas_output:seBPJ6NLJ2", "field": "vae"}},
            {"source": {"node_id": "core_metadata:aznHO0tknZ", "field": "metadata"}, "destination": {"node_id": "canvas_output:seBPJ6NLJ2", "field": "metadata"}}
        ]
    }

def wait_and_download(batch_id, expected_count):
    """
    Polls the batch status until complete, then downloads the latest generated images.
    """
    print("\n--- Waiting for Batch to Finish ---")
    status_url = f"{BASE_URL}/queue/default/b/{batch_id}/status"
    
    while True:
        try:
            resp = requests.get(status_url)
            status = resp.json()
            
            pending = status.get('pending', 0)
            in_progress = status.get('in_progress', 0)
            completed = status.get('completed', 0)
            failed = status.get('failed', 0)
            total = status.get('total', 0)
            
            print(f"\rPending: {pending} | In Progress: {in_progress} | Completed: {completed} | Failed: {failed}", end="")
            
            if pending == 0 and in_progress == 0:
                print("\nBatch processing finished!")
                break
                
            time.sleep(1) # Check every 1 second
            
        except Exception as e:
            print(f"\nError polling status: {e}")
            break

    # Once finished, retrieve images.
    # Note: InvokeAI doesn't provide a direct "batch_id -> image_id" list easily via HTTP API.
    # We will fetch the latest N images from the board.
    
    print("\n--- Downloading Images ---")
    if not os.path.exists(OUTPUT_DIR):
        os.makedirs(OUTPUT_DIR)
        
    # Fetch latest images
    images_url = f"{BASE_URL}/images/"
    params = {"limit": expected_count, "board_id": "none", "categories": "image"}
    
    try:
        # Get list of recent images
        img_resp = requests.get(images_url, params=params)
        images = img_resp.json().get("items", [])
        
        # Sort by creation time just in case, descending
        images.sort(key=lambda x: x['created_at'], reverse=True)
        
        # We only want the top N images where N = expected_count
        # (This assumes no one else is generating on this server at the exact same time)
        target_images = images[:expected_count]
        
        for img_meta in target_images:
            image_name = img_meta.get("image_name")
            image_url = img_meta.get("image_url") # Usually just the filename
            
            # The full download URL
            download_url = f"{BASE_URL}/images/i/{image_name}/full"
            
            print(f"Downloading {image_name}...")
            
            img_data = requests.get(download_url)
            with open(os.path.join(OUTPUT_DIR, image_name), 'wb') as f:
                f.write(img_data.content)
                
        print(f"Done! Saved {len(target_images)} images to ./{OUTPUT_DIR}/")
        
    except Exception as e:
        print(f"Error downloading images: {e}")

def main():
    try:
        prompts = load_prompts(PROMPTS_FILE)
        print(f"Loaded {len(prompts)} prompts.")
    except FileNotFoundError:
        print(f"Error: {PROMPTS_FILE} not found.")
        return

    # Generate seeds
    seeds = [random.randint(0, 2147483647) for _ in range(len(prompts))]
    
    # Construct batch
    batch_data = []
    batch_group = [
        {
            "node_path": "positive_prompt:35IPyJEmEK",
            "field_name": "value",
            "items": prompts
        },
        {
            "node_path": "seed:DVz4FhMt5E",
            "field_name": "value",
            "items": seeds
        }
    ]
    batch_data.append(batch_group)

    payload = {
        "prepend": False,
        "batch": {
            "graph": get_graph(),
            "runs": 1,
            "data": batch_data
        }
    }

    print("Sending batch to InvokeAI...")
    try:
        response = requests.post(INVOKE_URL, json=payload)
        response.raise_for_status()
        result = response.json()
        
        batch_id = result.get("batch", {}).get("batch_id")
        print(f"Success! Batch enqueued. Batch ID: {batch_id}")
        
        # New function call to wait and download
        wait_and_download(batch_id, len(prompts))
        
    except requests.exceptions.RequestException as e:
        print(f"Error sending request: {e}")
        if response.content:
            print("Server Response:", response.content.decode())

if __name__ == "__main__":
    main()