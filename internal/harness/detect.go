package harness

import "os"

func Detect(env Env) ([]DetectionResult, error) {
	results := make([]DetectionResult, 0, len(AllClients()))
	for _, client := range FileWriterClients() {
		configPath := client.ConfigPath(env)
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
		results = append(results, DetectionResult{
			ID:         client.ID,
			Name:       client.Name,
			ConfigPath: client.ConfigPath(env),
			PromptTier: true,
		})
	}

	return results, nil
}

func isDetected(client Client, env Env) (bool, error) {
	configExists, err := exists(client.ConfigPath(env))
	if err != nil || configExists {
		return configExists, err
	}
	return exists(markerPath(client, env))
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
