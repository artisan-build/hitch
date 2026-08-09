package harness

import "os"

func Detect(env Env) ([]DetectionResult, error) {
	results := make([]DetectionResult, 0, len(AllClients()))
	for _, client := range FileWriterClients() {
		configPath, err := client.ConfigPath(env)
		if err != nil {
			return nil, err
		}
		detected, err := isDetected(client, env)
		if err != nil {
			return nil, err
		}
		results = append(results, DetectionResult{
			ID:         client.ID,
			Name:       client.Name,
			ConfigPath: configPath,
			Detected:   detected,
		})
	}

	for _, client := range PromptTierClients() {
		detected, err := isDetected(client, env)
		if err != nil {
			return nil, err
		}
		results = append(results, DetectionResult{
			ID:         client.ID,
			Name:       client.Name,
			Detected:   detected,
			PromptTier: true,
		})
	}

	return results, nil
}

func isDetected(client Client, env Env) (bool, error) {
	if client.ConfigPath != nil {
		configPath, err := client.ConfigPath(env)
		if err != nil {
			return false, err
		}
		configExists, err := exists(configPath)
		if err != nil || configExists {
			return configExists, err
		}
	}
	markerPath, err := markerPath(client, env)
	if err != nil {
		return false, err
	}
	return exists(markerPath)
}

func exists(path string) (bool, error) {
	if path == "" {
		return false, nil
	}
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	if os.IsPermission(err) {
		return true, nil
	}
	return false, err
}
